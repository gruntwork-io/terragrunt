package redesign_test

import (
	"testing"
	"testing/synctest"
	"time"

	tea "charm.land/bubbletea/v2"
)

// settleTimeout is the maximum fake-clock duration driveModel advances after
// the test logic completes, to flush any pending timers (spinner ticks, paced
// loadFunc sleeps) so synctest can drain the bubble.
const settleTimeout = 10 * time.Second

// settleMaxIterations is a defensive cap on the drain loop in driveModel. The
// fake-clock deadline already bounds the loop in practice, but this guards
// against pathological cases where each iteration fails to advance the clock
// (e.g. a misbehaving cmd that re-arms itself instantly).
const settleMaxIterations = 10_000

// driveModel runs a synctest-friendly mini bubbletea loop against m. It is a
// drop-in stand-in for tea.NewProgram when tests want fake time instead of
// real wall-clock waits.
//
// The flow:
//  1. m.Init is dispatched, then the initial WindowSizeMsg.
//  2. Each entry in interact is delivered as a tea.Msg; cmds resulting from
//     Update are spawned in goroutines so synctest's fake clock advances when
//     they sleep.
//  3. When all goroutines block, the loop drains pending messages, sends the
//     next interact entry, and repeats. A QuitMsg ends the loop.
//
// The caller wraps this in synctest.Test so all goroutines live in one bubble.
func driveModel(t *testing.T, m tea.Model, width, height int, interact []tea.Msg) tea.Model {
	t.Helper()

	msgCh := make(chan tea.Msg, 100)

	spawn := func(cmd tea.Cmd) {
		if cmd == nil {
			return
		}

		go func() {
			msg := cmd()
			if msg == nil {
				return
			}

			msgCh <- msg
		}()
	}

	apply := func(msg tea.Msg) bool {
		if _, ok := msg.(tea.QuitMsg); ok {
			return false
		}

		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, c := range batch {
				spawn(c)
			}

			return true
		}

		var cmd tea.Cmd

		m, cmd = m.Update(msg)
		spawn(cmd)

		return true
	}

	spawn(m.Init())
	spawn(func() tea.Msg { return tea.WindowSizeMsg{Width: width, Height: height} })

	step := func() bool {
		// Loop until the bubble goes quiet: each iteration waits for
		// pending goroutines to block, drains any messages they produced,
		// and re-waits if apply spawned new cmds (a Batch fans out into
		// children; an Update returns a follow-up cmd).
		for {
			synctest.Wait()

			progress := false

			for {
				select {
				case msg := <-msgCh:
					progress = true

					if !apply(msg) {
						return false
					}

					continue
				default:
				}

				break
			}

			if !progress {
				return true
			}
		}
	}

	quit := false

	for _, msg := range interact {
		if !step() {
			quit = true
			break
		}

		if !apply(msg) {
			quit = true
			break
		}
	}

	if !quit {
		step()
	}

	// Drain background goroutines spawned during the test (recurring
	// timers, channel listeners) until the bubble goes quiet. time.Sleep
	// advances the fake clock so pending timers fire; we pull from msgCh
	// without dispatching to Update so cmd goroutines can send their
	// final message and exit.
	deadline := time.Now().Add(settleTimeout)
	iter := 0

	for iter = range settleMaxIterations {
		synctest.Wait()

		select {
		case <-msgCh:
			continue
		default:
		}

		if time.Now().After(deadline) {
			break
		}

		time.Sleep(50 * time.Millisecond)
	}

	if iter == settleMaxIterations-1 {
		t.Fatalf(
			"driveModel: bubble did not settle within %d iterations; "+
				"a cmd is likely re-arming itself instantly and "+
				"preventing the fake clock from advancing",
			settleMaxIterations,
		)
	}

	return m
}
