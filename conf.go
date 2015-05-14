package modern

import (
	"errors"
	"fmt"
	"log"

	js "github.com/rshmelev/go-json-light"

	// "strings"
	"time"
)

/*

Usage should be as simple as possible.
let's imagine we imported modernapp

conf := modernapp.NewConf("./", "myapp")

conf.Dyn().OptBool("debugMode")
conf.Local(). ...
conf.State(). ...

*/

//type ConfigUpdateHandler func()

type DynamicConfigurationUpdateHandler func(conf *ModernConf, olddyn js.IReadonlyObject)

type SimpleLogFunc func(params ...interface{})

type ModernConf struct {
	DevMode bool
	AppName string

	LocalConfFile string
	local         js.IObject

	dyn              *AutoLoadJSON
	DynConfUrl       string
	DynUpdatePeriod  time.Duration
	DynUpdateHandler DynamicConfigurationUpdateHandler
	// dyn             js.IObject
	// dynMutex        sync.Mutex

	StateFile       string
	StateSavePeriod time.Duration
	state           *js.SynchronizedObjectWrapper

	ConfLoadTimeout time.Duration

	Log      SimpleLogFunc
	ErrorLog SimpleLogFunc

	//--- aux

	lastDynBody      []byte
	failedLoadingDyn bool
}

type State struct {
}

func SetupConf(confdir string, basicConfiguration ...*ModernConf) *ModernConf {
	proto, _ := F.TheLastOf(&ModernConf{}, basicConfiguration).(*ModernConf)

	confdir = F.AppendSlash(confdir)

	if proto.DynConfUrl == "default" {
		proto.DynConfUrl = ""
	}
	F.EnsureNotEmptyString(&proto.DynConfUrl, confdir+"dyn.json")
	if proto.LocalConfFile == "default" {
		proto.LocalConfFile = confdir + "local.json"
	}
	// F.EnsureNotEmptyString(&proto.LocalConfFile, confdir+"local.json")
	if proto.StateFile == "default" {
		proto.StateFile = ""
	}
	F.EnsureNotEmptyString(&proto.StateFile, confdir+"state.json")

	F.EnsureNotEmptyDuration(&proto.DynUpdatePeriod, time.Second*5)
	F.EnsureNotEmptyDuration(&proto.ConfLoadTimeout, time.Second*15)
	F.EnsureNotEmptyDuration(&proto.StateSavePeriod, time.Second)

	if proto.Log == nil {
		proto.Log = func(params ...interface{}) {
			log.Println(params...)
		}
	}
	if proto.ErrorLog == nil {
		proto.ErrorLog = func(params ...interface{}) {
			log.Println("ERROR")
			log.Println(params...)
		}
	}

	return proto
}

func (c *ModernConf) Dyn() js.IReadonlyObject {
	return c.dyn.Data() // ToReadonlyObject()
}

func (c *ModernConf) Local() js.IReadonlyObject {
	return c.local.ToReadonlyObject()
}

func (c *ModernConf) State() js.IObject {
	return js.IObject(c.state)
}

func (c *ModernConf) SaveStateStep() {
	err := c.SaveState()
	if err != nil {
		c.ErrorLog("failed to save state to file: ", c.StateFile)
	}
	time.Sleep(c.StateSavePeriod)
	go c.SaveStateStep()
}

func (c *ModernConf) SaveState() error {
	b := c.state.ToByteArray(2)
	err := F.SafeWriteFile(c.StateFile, b)
	return err
}

func (c *ModernConf) LoadAll() error {

	if c.LocalConfFile != "" && c.LocalConfFile != "-" {
		var err error
		var code int

		c.Log("loading local configuration... ")
		c.local, err, code = js.NewObjectFromFile(c.LocalConfFile, c.ConfLoadTimeout)
		if err != nil {
			return err
		}
		if code != 200 {
			return errors.New(fmt.Sprint("got sad HTTP code ", code, " while loading local conf"))
		}
	}

	if c.StateFile != "-" && c.StateFile != "" {
		var err error

		c.Log("loading state... ")
		var prestate js.IObject
		prestate, err, _ = js.NewObjectFromFile(c.StateFile, c.ConfLoadTimeout)
		if err != nil {
			prestate = js.NewEmptyObject()
			c.ErrorLog("WARNING: failed to load state ("+c.StateFile+"), will start with clear state, error was: ", err)
		}
		c.state, _ = js.GetSynchronizedWrapper(prestate).(*js.SynchronizedObjectWrapper)
		c.Log("starting state saving loop... ")
		go c.SaveStateStep()
	}

	c.dyn = StartAutoLoadingJSON(c.DynConfUrl, c.DynUpdatePeriod, func(oldj, newj js.IReadonlyObject) {
		if c.DynUpdateHandler != nil {
			c.DynUpdateHandler(c, oldj)
		}
	}, false)

	c.Log("configuration has been loaded")

	return nil
}
