package modern

import (
	"bytes"
	"errors"
	"fmt"
	js "github.com/rshmelev/go-json-light"
	"log"
	"sync"
	"time"
)

type AutoLoadFileUpdatedHandler func(oldbytes, newbytes []byte)
type AutoLoadJSONUpdatedHandler func(oldj, newj js.IReadonlyObject)

type AutoLoadFile struct {
	Url           string
	UpdatePeriod  time.Duration
	UpdateHandler AutoLoadFileUpdatedHandler // protected with mutex

	mutex           sync.Mutex
	lastBody        []byte
	failedLoading   bool
	handleFirstTime bool
}

type AutoLoadJSON struct {
	autoLoadFile  *AutoLoadFile
	UpdateHandler AutoLoadJSONUpdatedHandler
	json          js.IObject
}

func (c *AutoLoadFile) Data() []byte {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.lastBody
}

func (c *AutoLoadFile) LoadNow() error {
	r := F.GetByteContents(c.Url, c.UpdatePeriod)
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if r.Err == nil && (r.Code == 200) {
		if c.failedLoading {
			log.Println("connection restored: " + c.Url)
		}
		c.failedLoading = false
		if bytes.Compare(r.Body, c.lastBody) != 0 {
			prevBody := c.lastBody
			c.lastBody = r.Body
			if c.UpdateHandler != nil {
				c.UpdateHandler(prevBody, c.lastBody)
			}
		}
		return nil
	}
	if r.Err != nil {
		return r.Err
	}
	return errors.New(fmt.Sprint("got error code ", r.Code, " while loading dyn conf"))
}

func (c *AutoLoadFile) LoadStep() {
	err := c.LoadNow()
	if err != nil {
		log.Println("ERROR: failed to load: ", c.Url)
	}
	time.Sleep(c.UpdatePeriod)
	go c.LoadStep()
}

func (c *AutoLoadFile) StartLoading() error {

	F.EnsureNotEmptyDuration(&c.UpdatePeriod, time.Second*5)

	if c.Url != "" && c.Url != "-" {
		var err error

		log.Println("trying to load: " + c.Url)
		temp := c.UpdateHandler
		if !c.handleFirstTime {
			c.UpdateHandler = nil
		}
		for {
			if err = c.LoadNow(); err == nil {
				break
			}
			log.Println("WARNING: failed to load: ", err)
			time.Sleep(time.Second * 2)
		}
		c.UpdateHandler = temp

		log.Println("starting autoload loop: " + c.Url)
		go c.LoadStep()
		return nil
	}
	return nil
}

//=======================================================================

func (c *AutoLoadJSON) Data() js.IReadonlyObject {
	c.autoLoadFile.mutex.Lock()
	defer c.autoLoadFile.mutex.Unlock()
	return c.json.ToReadonlyObject()
}

func StartAutoLoadingFile(url string, timeout time.Duration, handler AutoLoadFileUpdatedHandler, handleFirstTime bool) *AutoLoadFile {
	a := &AutoLoadFile{
		Url:             url,
		UpdatePeriod:    timeout,
		UpdateHandler:   handler,
		lastBody:        []byte{},
		handleFirstTime: handleFirstTime,
	}
	a.StartLoading()
	return a
}

func StartAutoLoadingJSON(url string, timeout time.Duration, handler AutoLoadJSONUpdatedHandler, handleFirstTime bool) *AutoLoadJSON {
	j := &AutoLoadJSON{json: js.NewObject(), UpdateHandler: handler}
	handle := handleFirstTime
	j.autoLoadFile = StartAutoLoadingFile(url, timeout, func(oldbytes, newbytes []byte) {
		predyn, err2 := js.NewObjectFromBytes(newbytes)
		if err2 != nil {
			log.Println("WARNING: failed to parse loaded json: " + url)
			return
		}
		log.Println("modification detected: " + url)
		olddyn := j.json
		j.json = predyn
		if j.UpdateHandler != nil && handle {
			j.UpdateHandler(olddyn, j.json.ToReadonlyObject())
		}
		handle = true
	}, true)

	return j
}
