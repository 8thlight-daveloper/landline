package agentd

import (
	"context"
	"fmt"
	"time"

	"github.com/8thlight-daveloper/landline/internal/model"
)

type Hub interface {
	Wakes(host string, waitSeconds int) ([]model.WakeJob, error)
	AckWake(id string, ack model.WakeAck) error
}

type Waker interface {
	Wake(target, text string) error
}

const (
	pollSeconds    = 30
	initialBackoff = time.Second
	maximumBackoff = 30 * time.Second
)

func Run(ctx context.Context, hub Hub, host string, wakers map[string]Waker, notify func(string)) error {
	return run(ctx, hub, host, wakers, notify, sleepFor)
}

func sleepFor(ctx context.Context, duration time.Duration) {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
	}
}

func run(ctx context.Context, hub Hub, host string, wakers map[string]Waker, notify func(string), sleep func(context.Context, time.Duration)) error {
	backoff := initialBackoff
	for ctx.Err() == nil {
		jobs, err := hub.Wakes(host, pollSeconds)
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			notify(fmt.Sprintf("could not poll the hub for wakes, retrying in %s: %v", backoff, err))
			sleep(ctx, backoff)
			backoff = min(backoff*2, maximumBackoff)
			continue
		}
		backoff = initialBackoff
		for _, job := range jobs {
			handle(hub, wakers, notify, job)
		}
	}
	return nil
}

func handle(hub Hub, wakers map[string]Waker, notify func(string), job model.WakeJob) {
	ack := model.WakeAck{OK: true}
	if waker := wakers[job.Kind]; waker == nil {
		ack = model.WakeAck{Error: fmt.Sprintf("this host has no wake method %s", job.Kind)}
	} else if err := waker.Wake(job.Target, job.Text); err != nil {
		ack = model.WakeAck{Error: err.Error()}
	}
	if ack.OK {
		notify(fmt.Sprintf("woke %s (%s), wake %s", job.Agent, job.Kind, job.ID))
	} else {
		notify(fmt.Sprintf("wake of %s (%s) failed, wake %s: %s", job.Agent, job.Kind, job.ID, ack.Error))
	}
	if err := hub.AckWake(job.ID, ack); err != nil {
		notify(fmt.Sprintf("could not acknowledge wake %s: %v", job.ID, err))
	}
}
