package modern

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"path"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/rshmelev/gologs/libgologs"

	"github.com/julienschmidt/httprouter"
	"github.com/rshmelev/easyws"
	"github.com/rshmelev/go-inthandler"
	js "github.com/rshmelev/go-json-light"
)

type PreroutingFunc func(r *http.Request) bool

type LoggingRedirector struct {
	elog libgologs.SomeLogger
	b    *bytes.Buffer
	ch   chan []byte
}

// TODO possible bug here
func CreateLoggingRedirector(baselogger libgologs.SomeLogger) *LoggingRedirector {
	x := &LoggingRedirector{
		elog: baselogger,
		b:    &bytes.Buffer{},
		ch:   make(chan []byte, 100),
	}

	reader := bufio.NewReader(x.b)

	go func() {
		for {
			a, ok := <-x.ch
			if !ok {
				break
			}
			//println(string(a))
			x.b.Write(a)
			s, _ := reader.ReadString('\n')
			x.elog.Error("http issue: " + s)
		}
	}()

	return x
}

func (ld *LoggingRedirector) Write(p []byte) (n int, err error) {
	ld.ch <- p
	return len(p), nil
}

func GetAccessLogHandler(elog libgologs.SomeLogger, excludePrefixes []string, excludeUrls map[string]bool) http.Handler {
	f := http.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uri := r.RequestURI
		for _, v := range excludePrefixes {
			if strings.HasPrefix(uri, v) {
				return
			}
		}
		if _, ok := excludeUrls[uri]; ok {
			return
		}

		elog.Info("HTTP: " + r.Method + " " + r.RequestURI)
	}))
	return f
}

//================================================================================= CONCAT HANDLERS

type ConcatHandlersStruct struct {
	handler, handler2 http.Handler
}

func (x *ConcatHandlersStruct) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	x.handler.ServeHTTP(w, r)

	// special case to prevent second handler from executing
	if r.Header.Get("!") != "" {
		return
	}

	x.handler2.ServeHTTP(w, r)
}

func ConcatHandlers(handler, handler2 http.Handler) http.Handler {
	s := &ConcatHandlersStruct{handler, handler2}
	return s
}

//======================================================================== REWRITER

type RewriteRule struct {
	regex   *regexp.Regexp
	replace string
}
type HttpRewriter struct {
	Rewrites []RewriteRule
}

func (hr *HttpRewriter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	for _, v := range hr.Rewrites {
		v.regex.ReplaceAllString(r.URL.Path, v.replace)
	}
}

//======================================================================== Websock LOGS handler

type WebsockLogHandler struct {
	Router      *httprouter.Router
	LogsUrlRoot string
	Logger      *libgologs.CoolLogger
}

func SetupWebsockLogHandler(h *WebsockLogHandler) {
	wss := easyws.SetupWsServer(&easyws.WsServer{
		Log: func(s string) {
			h.Logger.Info("WS: ", s)
		},
	}, &easyws.ConnectionConfig{
		OnMessage: func(c easyws.WebsocketTalker, b []byte) {

			//---- this part is for debugging and also is an example
			isSyncResponse, somejson, reply, _ := c.ExtractJSON(b)
			if isSyncResponse {
				return
			}
			if reply != nil {
				// calc response
				resp := somejson
				reply(resp)
				return
			}
			if somejson != nil {
				h.Logger.Info("WS.JSONMessage:", somejson)
				// do something?
				return
			}
			//---- end of this part is for debugging

			h.Logger.Info("WS.Messagee:", string(b))

		},
		OnConnectionStatusChanged: func(c easyws.WebsocketTalker, status int) {
			if status == easyws.STATUS_CONNECTED {
				history := h.Logger.GetHistory()
				c.SendJSON(history)
			}
			h.Logger.Info("WS: ", c.GetId(), " ", easyws.WsStatuses[status])
		},
	})
	go wss.Run()

	h.Router.GET(h.LogsUrlRoot, func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		wss.ServeHTTP(w, r)
	})
	h.Logger.WebsocketFunc = func(msg *libgologs.WebsocketLogMsg) {
		wss.BroadcastJSON(msg)
	}
}

//-----------------------------------------

func AttachSubdirFileServer(router *httprouter.Router, subdir string, webroot string) http.Handler {
	fileserver := http.FileServer(MakeJustFilesFs(webroot))
	//fileserver := http.FileServer(http.Dir(webroot))
	stripped := http.StripPrefix(subdir, fileserver)
	router.GET(subdir+"*some", func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		stripped.ServeHTTP(w, r)
	})
	return stripped
}

//=====================================================================================================================

func AttachHealthPointServer(router *httprouter.Router, url string, appName string, version string, dev bool) js.IObject {
	healthpoint := js.GetSynchronizedWrapper(js.NewEmptyObject())
	healthpoint.Put("app", appName)
	healthpoint.Put("version", version)
	healthpoint.Put("devmode", dev)
	healthpoint.Put("numcpu", runtime.NumCPU())
	started := time.Now().UTC()
	healthpoint.Put("started", started.Format("2006-01-02 15:04:05"))
	go func() {
		for _ = range time.Tick(time.Second) {
			elapsed := time.Since(started)
			healthpoint.Put("elapsed", elapsed.String())
			healthpoint.Put("elapsedsec", int(elapsed.Seconds()))
			healthpoint.Put("now", time.Now().UTC().Format("2006-01-02 15:04:05"))
		}
	}()
	router.GET(url, func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		// healthpoint.Put("goroutines", runtime.NumGoroutine())
		memstats := GetSomeMemStats()
		healthpoint.Put("memstats", memstats)
		w.Write(healthpoint.ToByteArray(2))
	})

	return healthpoint
}

func AttachHeapDumpHandler(router *httprouter.Router, heapDumpUrl, snapshotsFolder, DownloadPath string) {
	router.GET(heapDumpUrl, func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		filename := HeapDump(snapshotsFolder)
		url := F.AppendSlash(DownloadPath) + path.Base(filename)
		html := "<h1><a href='" + url + "'>" + url + "</a></h1>"
		w.Write([]byte(html))
	})
}

//=========================================================================================================== restartHandler

var restartHtmlTemplate = `
<html>
<head>
	<script src="//ajax.googleapis.com/ajax/libs/jquery/2.1.1/jquery.min.js"></script>
	<style> *{font-size: 20px; font-family: Verdana; margin: 10px;} </style>
</head>
<body>hopefully supervisor will restart me...
<script>
	var start = Date.now()
	function go() {
		$("body").append(".")
		$.ajax({ cache: false, url: "?startid=[RESTARTID]" })
			.done(function(msg) { if (msg != "different") setTimeout(go, 1000);
								  else 
								  	$("body").append("<div style='color: green'>RESTARTED in "+Math.round((Date.now()-start)/1000)+"s , please close this tab</div>") })
			.fail(function() { $("body").append("&nbsp;"); setTimeout(go, 1000) })
	}
	go()
</script>
</body>
</html>
`
var restartId string

func AttachShutdownHandler(router *httprouter.Router, url string) {
	router.GET(url, func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		qsid := r.URL.Query().Get("startid")
		if qsid == "" {
			restartId = fmt.Sprint(rand.Int63())
			html := strings.Replace(restartHtmlTemplate, "[RESTARTID]", restartId, 10)
			if strings.Contains(r.UserAgent(), "curl") {
				html = "ok"
			}
			reason := r.URL.Query().Get("reason")
			if reason == "" {
				reason = "unknown"
			}
			log.Println("got HTTP request to shut down with reason: " + reason)
			w.Write([]byte(html))
			go func() {
				time.Sleep(time.Second) // ensure response will reach the requestor
				gointhandler.InterruptTheApp()
			}()
		} else {
			ok := "equal"
			if qsid != restartId {
				ok = "different"
			}
			w.Write([]byte(ok))
		}
	})
}

//========================================================================================================

func SetupHttpServer(devmode bool, server *http.Server, elog libgologs.SomeLogger, preroutingFunc http.Handler) (*http.Server, *httprouter.Router, *HttpRewriter, error) {

	errorLogger := log.New(CreateLoggingRedirector(elog), "", 0)

	router := httprouter.New()
	preroutinghandler := ConcatHandlers(preroutingFunc, router)
	rewriter := &HttpRewriter{}
	mainhandler := ConcatHandlers(rewriter, preroutinghandler)

	server.Handler = mainhandler
	server.ErrorLog = errorLogger

	return server, router, rewriter, nil
}
