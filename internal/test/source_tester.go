// Package test contains test utilities.
package test

import (
	"context"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/unit"
)

// SourceTester is a static source tester.
type SourceTester struct {
	ctx       context.Context
	ctxCancel func()
	stream    *stream.Stream
	reader    stream.Reader

	Unit chan unit.Unit
	done chan struct{}
}

// NewSourceTester allocates a SourceTester.
func NewSourceTester(
	createFunc func(defs.StaticSourceParent) defs.StaticSource,
	resolvedSource string,
	conf *conf.Path,
) *SourceTester {
	ctx, ctxCancel := context.WithCancel(context.Background())

	t := &SourceTester{
		ctx:       ctx,
		ctxCancel: ctxCancel,
		Unit:      make(chan unit.Unit),
		done:      make(chan struct{}),
	}

	s := createFunc(t)

	go func() {
		s.Run(defs.StaticSourceRunParams{ //nolint:errcheck
			Context:        ctx,
			ResolvedSource: resolvedSource,
			Conf:           conf,
		})
		close(t.done)
	}()

	return t
}

// Close closes the tester.
func (t *SourceTester) Close() {
	t.ctxCancel()
	t.stream.RemoveReader(t.reader)
	<-t.done
}

// Log implements StaticSourceParent.
func (t *SourceTester) Log(_ logger.Level, _ string, _ ...interface{}) {
}

// SetReady implements StaticSourceParent.
func (t *SourceTester) SetReady(req defs.PathSourceStaticSetReadyReq) defs.PathSourceStaticSetReadyRes {
	t.stream, _ = stream.New(
		512,
		1460,
		req.Desc,
		req.GenerateRTPPackets,
		t,
		false,
	)

	t.reader = NilLogger

	t.stream.AddReader(t.reader, req.Desc.Medias[0], req.Desc.Medias[0].Formats[0], func(u unit.Unit) error {
		t.Unit <- u
		close(t.Unit)
		return nil
	})

	t.stream.StartReader(t.reader)

	return defs.PathSourceStaticSetReadyRes{
		Stream: t.stream,
	}
}

// SetNotReady implements StaticSourceParent.
func (t *SourceTester) SetNotReady(_ defs.PathSourceStaticSetNotReadyReq) {
}
