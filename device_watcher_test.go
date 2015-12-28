package goadb

import (
	"log"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/zach-klippenstein/goadb/util"
	"github.com/zach-klippenstein/goadb/wire"
)

func TestParseDeviceStatesSingle(t *testing.T) {
	states, err := parseDeviceStates(`192.168.56.101:5555	offline
`)

	assert.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, StateOffline, states["192.168.56.101:5555"])
}

func TestParseDeviceStatesMultiple(t *testing.T) {
	states, err := parseDeviceStates(`192.168.56.101:5555	offline
0x0x0x0x	device
`)

	assert.NoError(t, err)
	assert.Len(t, states, 2)
	assert.Equal(t, StateOffline, states["192.168.56.101:5555"])
	assert.Equal(t, StateOnline, states["0x0x0x0x"])
}

func TestParseDeviceStatesMalformed(t *testing.T) {
	_, err := parseDeviceStates(`192.168.56.101:5555	offline
0x0x0x0x
`)

	assert.True(t, util.HasErrCode(err, util.ParseError))
	assert.Equal(t, "invalid device state line 1: 0x0x0x0x", err.(*util.Err).Message)
}

func TestCalculateStateDiffsUnchangedEmpty(t *testing.T) {
	oldStates := map[string]DeviceState{}
	newStates := map[string]DeviceState{}

	diffs := calculateStateDiffs(oldStates, newStates)

	assert.Empty(t, diffs)
}

func TestCalculateStateDiffsUnchangedNonEmpty(t *testing.T) {
	oldStates := map[string]DeviceState{
		"1": StateOnline,
		"2": StateOnline,
	}
	newStates := map[string]DeviceState{
		"1": StateOnline,
		"2": StateOnline,
	}

	diffs := calculateStateDiffs(oldStates, newStates)

	assert.Empty(t, diffs)
}

func TestCalculateStateDiffsOneAdded(t *testing.T) {
	oldStates := map[string]DeviceState{}
	newStates := map[string]DeviceState{
		"serial": StateOffline,
	}

	diffs := calculateStateDiffs(oldStates, newStates)

	assertContainsOnly(t, []DeviceStateChangedEvent{
		DeviceStateChangedEvent{"serial", StateDisconnected, StateOffline},
	}, diffs)
}

func TestCalculateStateDiffsOneRemoved(t *testing.T) {
	oldStates := map[string]DeviceState{
		"serial": StateOffline,
	}
	newStates := map[string]DeviceState{}

	diffs := calculateStateDiffs(oldStates, newStates)

	assertContainsOnly(t, []DeviceStateChangedEvent{
		DeviceStateChangedEvent{"serial", StateOffline, StateDisconnected},
	}, diffs)
}

func TestCalculateStateDiffsOneAddedOneUnchanged(t *testing.T) {
	oldStates := map[string]DeviceState{
		"1": StateOnline,
	}
	newStates := map[string]DeviceState{
		"1": StateOnline,
		"2": StateOffline,
	}

	diffs := calculateStateDiffs(oldStates, newStates)

	assertContainsOnly(t, []DeviceStateChangedEvent{
		DeviceStateChangedEvent{"2", StateDisconnected, StateOffline},
	}, diffs)
}

func TestCalculateStateDiffsOneRemovedOneUnchanged(t *testing.T) {
	oldStates := map[string]DeviceState{
		"1": StateOffline,
		"2": StateOnline,
	}
	newStates := map[string]DeviceState{
		"2": StateOnline,
	}

	diffs := calculateStateDiffs(oldStates, newStates)

	assertContainsOnly(t, []DeviceStateChangedEvent{
		DeviceStateChangedEvent{"1", StateOffline, StateDisconnected},
	}, diffs)
}

func TestCalculateStateDiffsOneAddedOneRemoved(t *testing.T) {
	oldStates := map[string]DeviceState{
		"1": StateOffline,
	}
	newStates := map[string]DeviceState{
		"2": StateOffline,
	}

	diffs := calculateStateDiffs(oldStates, newStates)

	assertContainsOnly(t, []DeviceStateChangedEvent{
		DeviceStateChangedEvent{"1", StateOffline, StateDisconnected},
		DeviceStateChangedEvent{"2", StateDisconnected, StateOffline},
	}, diffs)
}

func TestCalculateStateDiffsOneChangedOneUnchanged(t *testing.T) {
	oldStates := map[string]DeviceState{
		"1": StateOffline,
		"2": StateOnline,
	}
	newStates := map[string]DeviceState{
		"1": StateOnline,
		"2": StateOnline,
	}

	diffs := calculateStateDiffs(oldStates, newStates)

	assertContainsOnly(t, []DeviceStateChangedEvent{
		DeviceStateChangedEvent{"1", StateOffline, StateOnline},
	}, diffs)
}

func TestCalculateStateDiffsMultipleChanged(t *testing.T) {
	oldStates := map[string]DeviceState{
		"1": StateOffline,
		"2": StateOnline,
	}
	newStates := map[string]DeviceState{
		"1": StateOnline,
		"2": StateOffline,
	}

	diffs := calculateStateDiffs(oldStates, newStates)

	assertContainsOnly(t, []DeviceStateChangedEvent{
		DeviceStateChangedEvent{"1", StateOffline, StateOnline},
		DeviceStateChangedEvent{"2", StateOnline, StateOffline},
	}, diffs)
}

func TestCalculateStateDiffsOneAddedOneRemovedOneChanged(t *testing.T) {
	oldStates := map[string]DeviceState{
		"1": StateOffline,
		"2": StateOffline,
	}
	newStates := map[string]DeviceState{
		"1": StateOnline,
		"3": StateOffline,
	}

	diffs := calculateStateDiffs(oldStates, newStates)

	assertContainsOnly(t, []DeviceStateChangedEvent{
		DeviceStateChangedEvent{"1", StateOffline, StateOnline},
		DeviceStateChangedEvent{"2", StateOffline, StateDisconnected},
		DeviceStateChangedEvent{"3", StateDisconnected, StateOffline},
	}, diffs)
}

func TestCameOnline(t *testing.T) {
	assert.True(t, DeviceStateChangedEvent{"", StateDisconnected, StateOnline}.CameOnline())
	assert.True(t, DeviceStateChangedEvent{"", StateOffline, StateOnline}.CameOnline())
	assert.False(t, DeviceStateChangedEvent{"", StateOnline, StateOffline}.CameOnline())
	assert.False(t, DeviceStateChangedEvent{"", StateOnline, StateDisconnected}.CameOnline())
	assert.False(t, DeviceStateChangedEvent{"", StateOffline, StateDisconnected}.CameOnline())
}

func TestWentOffline(t *testing.T) {
	assert.True(t, DeviceStateChangedEvent{"", StateOnline, StateDisconnected}.WentOffline())
	assert.True(t, DeviceStateChangedEvent{"", StateOnline, StateOffline}.WentOffline())
	assert.False(t, DeviceStateChangedEvent{"", StateOffline, StateOnline}.WentOffline())
	assert.False(t, DeviceStateChangedEvent{"", StateDisconnected, StateOnline}.WentOffline())
	assert.False(t, DeviceStateChangedEvent{"", StateOffline, StateDisconnected}.WentOffline())
}

func TestPublishDevicesRestartsServer(t *testing.T) {
	starter := &MockServerStarter{}
	dialer := &MockServer{
		Status: wire.StatusSuccess,
		Errs: []error{
			nil, nil, nil, // Successful dial.
			util.Errorf(util.ConnectionResetError, "failed first read"),
			util.Errorf(util.ServerNotAvailable, "failed redial"),
		},
	}
	watcher := deviceWatcherImpl{
		config:      ClientConfig{dialer},
		eventChan:   make(chan DeviceStateChangedEvent),
		startServer: starter.StartServer,
	}

	publishDevices(&watcher)

	assert.Empty(t, dialer.Errs)
	assert.Equal(t, []string{"host:track-devices"}, dialer.Requests)
	assert.Equal(t, []string{"Dial", "SendMessage", "ReadStatus", "ReadMessage", "Dial"}, dialer.Trace)
	err := watcher.err.Load().(*util.Err)
	assert.Equal(t, util.ServerNotAvailable, err.Code)
	assert.Equal(t, 1, starter.startCount)
}

type MockServerStarter struct {
	startCount int
	err        error
}

func (s *MockServerStarter) StartServer() error {
	log.Printf("Starting mock server")
	if s.err == nil {
		s.startCount += 1
		return nil
	} else {
		return s.err
	}
}

func assertContainsOnly(t *testing.T, expected, actual []DeviceStateChangedEvent) {
	assert.Len(t, actual, len(expected))
	for _, expectedEntry := range expected {
		assertContains(t, expectedEntry, actual)
	}
}

func assertContains(t *testing.T, expectedEntry DeviceStateChangedEvent, actual []DeviceStateChangedEvent) {
	for _, actualEntry := range actual {
		if expectedEntry == actualEntry {
			return
		}
	}
	assert.Fail(t, "expected to find %+v in %+v", expectedEntry, actual)
}