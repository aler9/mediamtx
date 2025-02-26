package recorder

import (
	"strings"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/recordstore"
)

type sample struct {
	*fmp4.PartSample
	dts int64
	ntp time.Time
}

type recorderInstance struct {
	rec *Recorder

	pathFormat string
	format     format
	skip       bool

	terminate chan struct{}
	done      chan struct{}
}

// Log implements logger.Writer.
func (ri *recorderInstance) Log(level logger.Level, format string, args ...interface{}) {
	ri.rec.Log(level, format, args...)
}

func (ri *recorderInstance) initialize() {
	ri.pathFormat = ri.rec.PathFormat

	ri.pathFormat = recordstore.PathAddExtension(
		strings.ReplaceAll(ri.pathFormat, "%path", ri.rec.PathName),
		ri.rec.Format,
	)

	ri.terminate = make(chan struct{})
	ri.done = make(chan struct{})

	switch ri.rec.Format {
	case conf.RecordFormatMPEGTS:
		ri.format = &formatMPEGTS{
			ri: ri,
		}
		ok := ri.format.initialize()
		ri.skip = !ok

	default:
		ri.format = &formatFMP4{
			ri: ri,
		}
		ok := ri.format.initialize()
		ri.skip = !ok
	}

	if !ri.skip {
		ri.rec.Stream.StartReader(ri)
	}

	go ri.run()
}

func (ri *recorderInstance) close() {
	close(ri.terminate)
	<-ri.done
}

func (ri *recorderInstance) run() {
	defer close(ri.done)

	if !ri.skip {
		select {
		case err := <-ri.rec.Stream.ReaderError(ri):
			ri.Log(logger.Error, err.Error())

		case <-ri.terminate:
		}

		ri.rec.Stream.RemoveReader(ri)
	} else {
		<-ri.terminate
	}

	ri.format.close()
}
