package asynq

import (
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/go-redis/redis/v7"
)

type poller struct {
	rdb *rdb

	// channel to communicate back to the long running "poller" goroutine.
	done chan struct{}

	// poll interval on average
	avgInterval time.Duration

	// redis ZSETs to poll
	zsets []string
}

func newPoller(rdb *rdb, avgInterval time.Duration, zsets []string) *poller {
	return &poller{
		rdb:         rdb,
		done:        make(chan struct{}),
		avgInterval: avgInterval,
		zsets:       zsets,
	}
}

func (p *poller) terminate() {
	// Signal the poller goroutine to stop polling.
	p.done <- struct{}{}
}

func (p *poller) start() {
	go func() {
		for {
			select {
			case <-p.done:
				fmt.Println("-------------[Poller]---------------")
				fmt.Println("Poller shutting down...")
				fmt.Println("------------------------------------")
				return
			default:
				p.exec()
				time.Sleep(p.avgInterval)
			}
		}
	}()
}

func (p *poller) exec() {
	for _, zset := range p.zsets {
		// Get next items in the queue with scores (time to execute) <= now.
		now := time.Now().Unix()
		msgs, err := p.rdb.zRangeByScore(zset, &redis.ZRangeBy{Min: "-inf", Max: strconv.Itoa(int(now))})
		if err != nil {
			log.Printf("radis command ZRANGEBYSCORE failed: %v\n", err)
			continue
		}
		fmt.Printf("[DEBUG] got %d tasks from %q\n", len(msgs), zset)

		for _, m := range msgs {
			if err := p.rdb.move(zset, m); err != nil {
				log.Printf("could not move task %+v to queue %q: %v", m, m.Queue, err)
				continue
			}
		}
	}
}
