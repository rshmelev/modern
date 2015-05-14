package modern

import (
	"flag"
	stdlog "log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/julienschmidt/httprouter"
	"github.com/kelseyhightower/envconfig"
	. "github.com/rshmelev/go-initstruct"
	interrupts "github.com/rshmelev/go-inthandler"
	js "github.com/rshmelev/go-json-light"
	. "github.com/rshmelev/go-ternary/if"
	"github.com/rshmelev/gologs/libgologs"
	"github.com/rshmelev/installasservice"
	"github.com/rshmelev/restarter/librestarter"
	//"github.com/spacemonkeygo/monitor" <- does not compile for 386
)

/*
	your app should look like this:

	func main() {
		.... := TrivialSetup(...)
		... configure routes ...
		TrivialStart(...)
		... your code
	}
*/

/*
	trivial features:
	- call AppDir
	- use all cores + randomize
	- interrupts handler for graceful shutdown
	- env variables support and parsing .env file using godotenv + envconfig
	- http server with julienschmidt/httprouter and websocket support rshmelev/easyws
	- supports __installservice and __phoenix options using rshmelev/librestarter and rshmelev/installasservice
	- tracking mem stats + ability to fetch them via http
	- logging to file(+daily rotate using modified lumberjack)
	  with history for remote websocket log viewing + setting as std logger
	- setting up json configs (dynamic loading with http fetching support, local json, state saving)
	- http features: /healthpoint for mem stats and other info, /static file serving,
	  dumping heap, smart app restart, ...
*/

// testing reminder
// https://github.com/smartystreets/goconvey/wiki/Assertions

func UseAllCores() {
	runtime.GOMAXPROCS(runtime.NumCPU())
}

// for debugging
func HeapDump(path string) string {
	filename := f.AppendSlash(path) + "heapdump" + time.Now().UTC().Format("-2006-01-02-15-04-05")
	ff, err := os.Create(filename)
	if err == nil {
		debug.WriteHeapDump(ff.Fd())
	}
	return filename
}

// ease of use of "useful functions"
var f = F

// we configure github.com/rshmelev/go-inthandler to use these things
var Stop bool = false
var StopChannel chan struct{}

// everything in one place
type TrivialSetupConf struct {
	AppName     string
	CompanyName string
	CompanySite string

	Version         string
	GoVersion       string
	BuildTime       string
	CodeRev         string
	ModifiedSources string

	HttpBind string

	StaticContentRoot string `init:"/static/"`
	WebsocketLogsRoot string
	MonitorsUrl       string
	HeapDumpUrl       string
	KillUrl           string

	// these options have default values
	StaticContentRootURL string
	HealthPointURL       string `init:"/infohub"`
	LogsPath             string `init:"{{ .AppDir }}/logs"`
	ConfPath             string `init:"{{ .AppDir }}/conf"`

	StateFile     string `init:"default"`
	LocalConfFile string
	DynConfUrl    string

	IsForProduction bool
}

// company + appname + version + buildtime
var FullAppString = ""
var StdLogFlags = stdlog.Lshortfile
var AppDir = "."
var Debug = false

func TrivialSetup(envconf interface{}, c *TrivialSetupConf) (libgologs.SomeLogger, *ModernConf, *httprouter.Router, *http.Server, js.IObject) {

	if IsForProductionRequest() {
		AnswerIsForProduction(c.IsForProduction)
	}

	if matched, err := regexp.Match("go-build\\d+.+", []byte(os.Args[0])); !matched && err == nil {
		if appdir, err := filepath.Abs(filepath.Dir(os.Args[0])); err == nil {
			AppDir = appdir
		}
	}

	InitZeroFieldsRecursively(c)

	Debug = false
	rand.Seed(time.Now().UTC().UnixNano())

	fullname := c.CompanyName + "-" + c.AppName
	if c.CompanyName == "" {
		fullname = c.AppName
	}

	FullAppString = fullname + " v" + c.Version +
		If(!c.IsForProduction).Then(" (DEBUG)").Else("").Str()
	FullAppString += If(c.BuildTime != "").Then(" built on " + c.BuildTime).Else("").Str()
	FullAppString += If(c.GoVersion != "").Then(" (" + c.GoVersion + ")").Else("").Str()

	if c.CodeRev != "" && len(c.CodeRev) > 6 {
		mod := ""
		if c.ModifiedSources != "" {
			mods := strings.Split(c.ModifiedSources, "\n")
			firstmods := mods
			if len(mods) > 3 {
				firstmods = mods[:3]
				mods = mods[3:]
			} else {
				mods = []string{}
			}
			mod = strings.Join(firstmods, ", ") +
				If(len(mods) > 0).Then(
					" and "+strconv.Itoa(len(mods))+" more").Else("").Str()
			if len(mod) > 0 {
				mod = " + modified " + mod
			}
		}

		FullAppString += " (rev=" + c.CodeRev[:6] + mod + ")"
	}

	//_ = godotenv.Load("sample.env")
	_ = godotenv.Load(".env")                                   // default godotenv way
	_ = godotenv.Load(c.AppName + ".env")                       // myapp.env
	_ = godotenv.Load(c.CompanyName + "-" + c.AppName + ".env") // mycompany-myapp.env
	_ = godotenv.Load(c.ConfPath + "/.env")                     // however modernconf stores configs in /conf folder by default...
	_ = godotenv.Load(c.ConfPath + "/" + c.AppName + ".env")    // however modernconf stores configs in /conf folder by default...
	_ = godotenv.Load("gitignored.env")                         // something should be definitely gitignored

	//TrivialSetupConf
	if enverr := envconfig.Process(c.AppName, c); enverr != nil {
		stdlog.Fatalln("envconfig.Process of TrivialSetupConf failed: ", enverr)
	}

	// cool simple autoreplace of {{ .AppDir }}
	c_map := js.StructToMapOrDie(c)
	c_new, _ := js.NewObjectFromString(replaceAppDir(c_map.ToString()))
	c_new.FillStruct(&c)

	flag.Bool("v", false, "get version")
	fullLogsPath := flag.String("logto", c.LogsPath+"/"+fullname+".log", "full path of rotating log")
	flag.Parse()
	probablyOutputVersion()

	installasservice.ProbablyInstallAsService(&installasservice.ServiceInstallerOptions{
		MaxShutdownTime: interrupts.MaxTimeToWaitForCleanup,
		AppName:         c.AppName,
		CompanyName:     c.CompanyName,
	})

	interrupts.StopPointer = &Stop
	StopChannel = interrupts.StopChannel
	librestarter.ProbablyBecomeRestarter(librestarter.RestarterOptions{
		ShutdownURL:             "http://" + c.HttpBind + c.KillUrl,
		MaxTimeToWaitForCleanup: &interrupts.MaxTimeToWaitForCleanup,
		Stop:        interrupts.StopPointer,
		StopChannel: interrupts.StopChannel,
	})

	Debug = os.Getenv(strings.ToUpper(c.AppName)+"_DEVMODE") == "1"
	dev := Debug

	UseAllCores()
	TrackMemStats()

	if c.LogsPath != "-" {
		if err := os.MkdirAll(c.LogsPath, 0755); err != nil {
			stdlog.Fatalln("cannot create logs folder: " + c.LogsPath)
		}
	} else {
		*fullLogsPath = ""
	}
	logconf := &libgologs.CoolLogger{
		FullLogFilename: *fullLogsPath,
		MemoryLimit:     5000,
	}
	log := libgologs.NewCoolLogger(logconf) //defer log.Flush()
	log.SetAsStdLogWriter(StdLogFlags)
	stdlog.Println("std log package integration check...")

	log.Info("starting " + FullAppString)
	if c.CompanySite != "" {
		log.Info("get more info about " + c.AppName + " at http://" + c.CompanySite)
	}
	if !c.IsForProduction {
		LogNotForProductionMessage()
	}

	mconf := SetupConf(c.ConfPath, &ModernConf{
		DevMode:       dev,
		Log:           log.Info,
		ErrorLog:      log.Error,
		StateFile:     c.StateFile,
		LocalConfFile: c.LocalConfFile,
		DynConfUrl:    c.DynConfUrl,
	})

	if envconf != nil {
		if e := envconfig.Process(c.AppName, envconf); e != nil {
			log.Error("envconfig.Process failed: ", e)
			os.Exit(1)
		}
	}

	// TODO: what if i do not want http server?
	s := &http.Server{
		Addr: c.HttpBind,
	}

	server, router, _, _ := SetupHttpServer(dev, s, log,
		GetAccessLogHandler(log,
			[]string{c.StaticContentRootURL},
			map[string]bool{c.HealthPointURL: true, c.MonitorsUrl: true, c.WebsocketLogsRoot: true}))

	if good(c.WebsocketLogsRoot) {
		SetupWebsockLogHandler(&WebsockLogHandler{Router: router, LogsUrlRoot: c.WebsocketLogsRoot, Logger: logconf})
	}

	if good(c.HeapDumpUrl) {
		snapshotsFolder := f.AppendSlash(c.StaticContentRoot) + "heapdumps"
		os.MkdirAll(snapshotsFolder, 755)
		AttachHeapDumpHandler(router, c.HeapDumpUrl, snapshotsFolder, c.StaticContentRootURL+"heapdumps")
	}

	if good(c.KillUrl) {
		AttachShutdownHandler(router, c.KillUrl)
	}

	AttachSubdirFileServer(router, c.StaticContentRootURL, c.StaticContentRoot)

	if good(c.MonitorsUrl) {
		router.GET(c.MonitorsUrl, func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
			//monitor.DefaultStore.ServeHTTP(w, r)
			w.Write([]byte("monitors not available"))
		})
	}

	healthpoint := AttachHealthPointServer(router, c.HealthPointURL, fullname, c.Version, dev)
	healthpoint.Put("buildtime", c.BuildTime)

	return log, mconf, router, server, healthpoint
}

func good(s string) bool {
	return s != "" && s != "-"
}

// call TrivialStart after you finish configuration of logger, server, router, etc. -
func TrivialStart(log libgologs.SomeLogger, conf *ModernConf, router *httprouter.Router, server *http.Server) error {
	err := conf.LoadAll()
	if err != nil {
		log.Error(err)
		log.Flush()
		os.Exit(1)
	}

	go func() {
		log.Info("HTTP server is listening ", server.Addr)
		err := server.ListenAndServe()
		if err != nil {
			log.Error("HTTP Server failed with error: ", err)
			interrupts.InterruptTheApp()
		}
	}()

	// before this moment, better to have some ctrl+c
	interrupts.TakeCareOfInterrupts(false)

	return nil
}

func probablyOutputVersion() {
	if len(os.Args) > 1 && os.Args[1] == "-v" {
		println(FullAppString)
		os.Exit(0)
	}
}

func env(s, appname string) string {
	s = strings.Replace(s, "${", "${"+strings.ToUpper(appname)+"_", -1)
	return os.ExpandEnv(s)
}

func replaceAppDir(s string) string {
	d := strings.Replace(AppDir, "\\", "\\\\", -1)
	return strings.Replace(s, "{{ .AppDir }}", d, -1)
}

func LogNotForProductionMessage() {
	stdlog.Println("")
	stdlog.Println("")
	stdlog.Println("*******************************************************")
	stdlog.Println("*******************************************************")
	stdlog.Println("")
	stdlog.Println("          THIS BUILD IS NOT FOR PRODUCTION !")
	stdlog.Println("")
	stdlog.Println("*******************************************************")
	stdlog.Println("*******************************************************")
	stdlog.Println("")
	stdlog.Println("")
}

func IsForProductionRequest() bool {
	args := os.Args
	q := false
	for _, v := range args {
		if strings.HasPrefix(v, "__is_for_production") {
			q = true
		}
	}
	return q
}

func AnswerIsForProduction(isit bool) {
	if isit {
		println("true")
		os.Exit(0)
	} else {
		println("false")
		os.Exit(1)
	}
}

func init() {

	//fmt.Println("modernapp.init")
}
