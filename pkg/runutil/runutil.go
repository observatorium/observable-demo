package runutil

import (
	"io"
	"io/ioutil"
	"log"
)

// ExhaustCloseWithLogOnErr closes the io.ReadCloser with a log message on error but exhausts the reader before.
func ExhaustCloseWithLogOnErr(rc io.ReadCloser) {
	if _, err := io.Copy(ioutil.Discard, rc); err != nil {
		log.Printf("error: failed to exhaust reader, performance may be impeded, err: %v\n", err)
	}

	if err := rc.Close(); err != nil {
		log.Printf("error: failed to exhaust reader, performance may be impeded, err: %v\n", err)
		return
	}
}
