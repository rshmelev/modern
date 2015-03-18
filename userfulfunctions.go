package modern

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"reflect"
	"strconv"
	//	js "github.com/rshmelev/go-json-light"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

type UsefulFunctions struct {
	TraceMode bool
}

func (_ *UsefulFunctions) AppendSlash(str string) string {
	if strings.HasSuffix(str, "/") {
		return str
	}
	return str + "/"
}

func (f *UsefulFunctions) RegexExtract(rx, str string) (string, error) {
	r, err := regexp.Compile(rx)
	if err != nil {
		return "", err
	}
	sm := r.FindStringSubmatch(str)
	if len(sm) > 0 {
		return sm[1], nil
	}
	return "", errors.New("group unavailable")
}

func (f *UsefulFunctions) RegexReplace(source, rx, repl string) (string, error) {
	r, err := regexp.Compile(rx)
	if err != nil {
		return "", err
	}
	res := r.ReplaceAllString(source, repl)
	return res, nil
}

func (_ *UsefulFunctions) FindKeyByValue(m map[string]string, v string) (string, error) {
	for i, vv := range m {
		if vv == v {
			return i, nil
		}
	}
	return "", errors.New("Key not found for value: " + v)
}

func (f *UsefulFunctions) Trace(params ...interface{}) {
	if !f.TraceMode {
		return
	}

	print("\nTRACE: ")
	for _, v := range params {
		fmt.Printf("%+v", v)
	}
	println("\n")

}

func (f *UsefulFunctions) GetContents(url string, timeout time.Duration) *HttpStringResponse {
	bc := f.GetByteContents(url, timeout)
	if bc.Body != nil {
		return &HttpStringResponse{string(bc.Body), bc.Err, bc.Code}
	}
	return &HttpStringResponse{"", bc.Err, bc.Code}
}

type HttpByteResponse struct {
	Body []byte
	Err  error
	Code int
}

func (r *HttpByteResponse) ToStringOrError() string {
	if r == nil {
		return "<NIL>"
	}
	if r.Err != nil {
		return r.Err.Error()
	}
	if r.Code != 200 {
		return strconv.Itoa(r.Code)
	}
	return string(r.Body)
}

type HttpStringResponse struct {
	Body string
	Err  error
	Code int
}

func (f *UsefulFunctions) GetByteContents(url string, timeout time.Duration) *HttpByteResponse {
	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		client := http.Client{
			Timeout:   timeout,
			Transport: tr,
		}
		r, err := client.Get(url)
		defer func() {
			if r != nil && r.Body != nil {
				r.Body.Close()
			}
		}()

		//fmt.Printf("%#v\n", r)

		if err != nil {
			return &HttpByteResponse{nil, err, 0}
		}

		x, err := ioutil.ReadAll(r.Body)

		return &HttpByteResponse{x, nil, r.StatusCode}
	} else {
		bytes, err := ioutil.ReadFile(url)
		if err != nil {
			switch {
			case os.IsNotExist(err):
				return &HttpByteResponse{nil, err, 404}
			case os.IsPermission(err):
				return &HttpByteResponse{nil, err, 403}
			default:
				return &HttpByteResponse{nil, err, 500}
			}
		}
		return &HttpByteResponse{bytes, nil, 200}
	}
}

func (f *UsefulFunctions) SafeWriteFile(filename string, data []byte) error {
	tempfilename := filename + "_temp_" + time.Now().UTC().Format("2006-01-02_15-04-05")
	tempfilenameold := tempfilename + ".old"
	tempfilename += ".new"

	// write new data to temp file 1
	e := ioutil.WriteFile(tempfilename, data, 600)
	if e != nil {
		return e
	}

	// move current file to temp file 2
	e = os.Rename(filename, tempfilenameold)
	if e != nil && !os.IsNotExist(e) {
		// clean the garbage
		os.Remove(tempfilename)
		return e
	}

	// move temp file 1 to current file
	e = os.Rename(tempfilename, filename)
	if e != nil {
		// attempt to restore previous file state
		os.Rename(tempfilenameold, filename)
		return e
	}

	// now we can finally remove old file
	e = os.Remove(tempfilenameold)
	if os.IsNotExist(e) {
		return nil
	}

	return e
}

func (f *UsefulFunctions) RandInt(min int, max int) int {
	return min + rand.Intn(max-min)
}

var alphanumBytes []byte = []byte("0123456789abcdefghijklmnopqrstuvwxyz")

func (f *UsefulFunctions) RandomAlphaNumString(l int) string {
	return f.RandomStringFromBytes(l, alphanumBytes)
}

func (f *UsefulFunctions) RandomStringFromBytes(l int, b []byte) string {
	bytes := make([]byte, l)
	vlen := len(b)
	for i := 0; i < l; i++ {
		bytes[i] = b[rand.Intn(vlen)]
	}
	return string(bytes)
}

func (f *UsefulFunctions) RandomStringFromVariants(l int, variants []string) string {
	buf := &bytes.Buffer{}
	vlen := len(variants)
	for i := 0; i < l; i++ {
		buf.WriteString(variants[rand.Intn(vlen)])
	}
	return buf.String()
}

func (f *UsefulFunctions) JsonClone(oin, oout interface{}) error {
	b, err := json.Marshal(oin)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, oout)
}

func (f *UsefulFunctions) ToJsonString(oin interface{}) string {
	return string(f.ToJsonBytes(oin))
}

func (f *UsefulFunctions) ToJsonBytes(oin interface{}) []byte {
	b, err := json.Marshal(oin)
	if err != nil {
		return []byte("{}")
	}
	return b
}

func (f *UsefulFunctions) MultiSelect(channels ...interface{}) (int, interface{}) {
	if len(channels) == 0 {
		return -1, nil
	}
	cases := make([]reflect.SelectCase, len(channels))
	for i, ch := range channels {
		cases[i] = reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(ch)}
	}
	chosen, value, _ := reflect.Select(cases)
	// ok will be true if the channel has not been closed.
	return chosen, value.Interface()
}

func (f *UsefulFunctions) Sleep(dur time.Duration, wakeupchannels ...interface{}) bool {
	if len(wakeupchannels) == 0 {
		time.Sleep(dur)
		return false
	} else {
		timer := time.NewTimer(dur)
		wakeupchannelsslice := append(wakeupchannels, timer.C)
		i, _ := f.MultiSelect(wakeupchannelsslice...)
		return i != (len(wakeupchannels) - 1)
	}
}

//----------------------------------

func (f *UsefulFunctions) Milliseconds(mul int) time.Duration {
	return time.Duration(time.Millisecond * time.Duration(mul))
}

var F = &UsefulFunctions{}

//-----------------------------------------------

// simple file listing will outline public ip of the machine
func (f *UsefulFunctions) GenPublicIpFile() string {
	b := f.GetContents("http://api.ipify.org/", time.Second)
	if b.Code == 200 && b.Err == nil {
		b.Body, _ = f.RegexReplace(b.Body, "[^0-9.]", "")
		ioutil.WriteFile(b.Body, []byte(b.Body), 0777)
	}
	return b.Body
}

//

//---------------------------

func (f *UsefulFunctions) LastOf(t interface{}) interface{} {
	switch reflect.TypeOf(t).Kind() {
	case reflect.Slice:
		s := reflect.ValueOf(t)
		return s.Index(s.Len() - 1).Interface()
	}
	return nil
}
func (f *UsefulFunctions) TheLastOf(first interface{}, params interface{}) interface{} {
	z := f.LastOf(params)
	if z == nil {
		return first
	}
	return z
}

func (f *UsefulFunctions) EnsureNotEmptyInt(a *int, _default int) {
	*a = f.OptInt(*a, _default)
}
func (f *UsefulFunctions) EnsureNotEmptyFloat(a *float64, _default float64) {
	*a = f.OptFloat(*a, _default)
}
func (f *UsefulFunctions) EnsureNotEmptyString(a *string, _default string) {
	*a = f.OptString(*a, _default)
}
func (f *UsefulFunctions) EnsureNotEmptyTime(a *time.Time, _default time.Time) {
	*a = f.OptTime(*a, _default)
}
func (f *UsefulFunctions) EnsureNotEmptyDuration(a *time.Duration, _default time.Duration) {
	*a = f.OptDuration(*a, _default)
}

func (f *UsefulFunctions) OptInt(a, _default int) int {
	if a == 0 {
		return _default
	}
	return a
}

func (f *UsefulFunctions) OptString(a, _default string) string {
	if a == "" {
		return _default
	}
	return a
}

func (f *UsefulFunctions) OptFloat(a, _default float64) float64 {
	if a == 0.0 {
		return _default
	}
	return a
}

func (f *UsefulFunctions) OptTime(a, _default time.Time) time.Time {
	if a.IsZero() {
		return _default
	}
	return a
}

func (f *UsefulFunctions) OptDuration(a, _default time.Duration) time.Duration {
	if a == time.Duration(0) {
		return _default
	}
	return a
}
