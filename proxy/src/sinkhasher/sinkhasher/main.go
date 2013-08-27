package main

import (
	"flag"
	"fmt"
	"github.com/cloudfoundry/loggregatorlib/cfcomponent"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
)

const (
	TAIL_PATH     = "/tail/"
	DUMP_PATH     = "/dump/"
	versionNumber = `0.0.TRAVIS_BUILD_NUMBER`
	gitSha        = `TRAVIS_COMMIT`
)

type Config struct {
	Host         string
	Loggregators []string
}

var (
	logFilePath = flag.String("logFile", "", "The agent log file, defaults to STDOUT")
	logLevel    = flag.Bool("v", false, "Verbose logging")
	version     = flag.Bool("version", false, "Version info")
	configFile  = flag.String("config", "config/loggregator_proxy.json", "Location of the loggregator proxy config json file")
)

func main() {
	//	logger := cfcomponent.NewLogger(*logLevel, *logFilePath, "udprouter")
	flag.Parse()

	if *version {
		fmt.Printf("\n\nversion: %s\ngitSha: %s\n\n", versionNumber, gitSha)
		return
	}

	config := &Config{Host: "0.0.0.0:8080"}
	err := cfcomponent.ReadConfigInto(config, *configFile)
	if err != nil {
		panic(err)
	}

	rp := &httputil.ReverseProxy{Director: dump_director}
	http.Handle(DUMP_PATH, rp)
	http.Handle(TAIL_PATH, websocketProxy("0.0.0.0:8081"))
	err = http.ListenAndServe("0.0.0.0:8080", nil)
	if err != nil {
		panic(err)
	}
}

func dump_director(r *http.Request) {
	r.URL.Host = "0.0.0.0:8081"
	r.URL.Scheme = "http"
}

func websocketProxy(target string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d, err := net.Dial("tcp", target)
		if err != nil {
			http.Error(w, "Error contacting backend server.", 500)
			log.Printf("Error dialing websocket backend %s: %v", target, err)
			return
		}
		hj, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "Not a hijacker?", 500)
			return
		}
		nc, _, err := hj.Hijack()
		if err != nil {
			log.Printf("Hijack error: %v", err)
			return
		}
		defer nc.Close()
		defer d.Close()

		err = r.Write(d)
		if err != nil {
			log.Printf("Error copying request to target: %v", err)
			return
		}

		errc := make(chan error, 2)
		cp := func(dst io.Writer, src io.Reader) {
			_, err := io.Copy(dst, src)
			errc <- err
		}
		go cp(d, nc)
		go cp(nc, d)
		<-errc
	})
}
