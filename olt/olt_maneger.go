package olt

import (
	"context"
	"io"
	"log/slog"
	"runtime"
	"sync"
	"time"

	"sirherobrine23.com.br/Sirherobrine23/zh-volt/sources"
	"sirherobrine23.com.br/Sirherobrine23/zh-volt/util"
)

type OltManeger struct {
	Log       *slog.Logger
	StartTime time.Time

	context   context.Context
	pktSource sources.Sources
	newDev    chan struct{}
	olt       *util.SyncMap[string, *Olt]
	onceStart func()
}

// Return new OLT Manager
func NewOltProcess(ctx context.Context, source sources.Sources, slogLevel slog.Level, logWrite io.Writer) (oltManeger *OltManeger) {
	oltManeger = &OltManeger{
		context:   ctx,
		pktSource: source,
		Log:       slog.New(slog.NewJSONHandler(logWrite, &slog.HandlerOptions{Level: slogLevel})),
		newDev:    make(chan struct{}, 1),
		olt:       util.NewSyncMap[string, *Olt](),
		onceStart: sync.OnceFunc(func() {
			cores := runtime.NumCPU() * 2
			for i := range cores {
				go oltManeger.pkts(i)
			}

			// Send fist Broadcast ping to get all OLTs in local area
			oltManeger.StartTime = time.Now()
			oltManeger.pktSource.Send(sources.New().SetRequestType(0xc).SetFlag2(0xff))
		}),
	}
	source.Slog(oltManeger.Log)
	return oltManeger
}

// Get Current OLTs
func (man OltManeger) Olts() map[string]*Olt { return man.olt.Clone() }

// Process packets in background
func (man *OltManeger) Start() { man.onceStart() }

func (maneger *OltManeger) Close() (err error) {
	if maneger.pktSource != nil {
		if err = maneger.pktSource.Close(); err != nil {
			return
		}
	}
	if maneger.newDev != nil {
		select {
		case <-maneger.newDev:
		default:
			close(maneger.newDev)
			maneger.newDev = nil
		}
	}
	return
}

// Wait for fist device locate
func (man *OltManeger) Wait(timeout time.Duration) bool {
	if man.newDev == nil {
		return true
	}

	select {
	case <-man.newDev:
		return true
	case <-time.After(timeout):
		man.Close()
		return false
	}
}

func (man *OltManeger) pkts(workerID int) {
	defer man.Close()
	log := slog.New(man.Log.Handler().WithAttrs([]slog.Attr{slog.Int("id", workerID+1)}))
	log.Log(man.context, slog.LevelInfo, "Starting process packet")

	for pkt := range man.pktSource.GetPkts() {
		if pkt.Error != nil {
			log.Error("Error on process packet", "error", pkt.Error)
			continue
		}
		log.Debug("Pkt received", "data", pkt.Data)

		olt, exist := man.olt.Get(pkt.Mac.String())
		if !exist {
			timeoutResponse := time.Since(man.StartTime)
			olt = NewOlt(man, pkt.Mac, timeoutResponse)
			log.Info("New olt",
				"Mac address", pkt.Mac,
				"response time", olt.Response.String(),
				"timeout", olt.TimeoutForResponse.String())

			man.olt.Set(pkt.Mac.String(), olt)
			if man.newDev != nil {
				man.newDev <- struct{}{}
				close(man.newDev)
				man.newDev = nil
			}
		}

		olt.Packet(pkt)
	}
}
