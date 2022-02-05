package executor

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ikenchina/octopus/common/slice"
	"github.com/ikenchina/octopus/define"
)

// rmClientMock
type rmClientMock struct {
	servers *rmMock
}

func (pc *rmClientMock) start(commit <-chan *ActionNotify) {
	go func() {
		for cc := range commit {
			pc.action(cc)
		}
	}()
}

func (pc *rmClientMock) action(sn *ActionNotify) {
	var bodyStruct *rmBodyMock
	if slice.InSlice(sn.BranchType, define.BranchTypeCommit, define.BranchTypeCancel, define.BranchTypeCompensation, define.BranchTypeConfirm) {
		bodyStruct = &rmBodyMock{}
	}

	body, err := pc.servers.Execute(sn.Ctx, sn.txn.Gtid, sn.Action, sn.Payload, sn.txn.State, bodyStruct)
	sn.Done(err, body)
}

// rmMock
type rmMock struct {
	handlers map[string]*rmMockHandler
}

func (rmm *rmMock) getHandler(name, branch string) *rmMockHandler {
	for _, h := range rmm.handlers {
		if h.name == name && h.branch == branch {
			return h
		}
	}
	return nil
}
func (rmm *rmMock) getHandlerByAction(action string) *rmMockHandler {
	for u, h := range rmm.handlers {
		if u == action {
			return h
		}
	}
	return nil
}

type rmMockHandler struct {
	sync.RWMutex
	name           string
	branch         string
	url            string
	requestTimeout time.Duration
	count          int32
	stores         map[string]*rmBodyMock
}

func (rm *rmMockHandler) timeout() time.Duration {
	rm.RLock()
	defer rm.RUnlock()
	return rm.requestTimeout
}

func (rm *rmMockHandler) setTimeout(t time.Duration) {
	rm.Lock()
	defer rm.Unlock()
	rm.requestTimeout = t
}

func (rm *rmMockHandler) requestCount() int {
	return int(atomic.LoadInt32(&rm.count))
}

func (rm *rmMockHandler) getNotify(gtid string) *rmBodyMock {
	r, ok := rm.stores[gtid]
	if !ok {
		return nil
	}
	return r
}

func newRmMock(handlers map[string]*rmMockHandler) *rmMock {
	pm := &rmMock{
		handlers: handlers,
	}
	for u, h := range pm.handlers {
		h.url = u
	}
	return pm
}

type rmBodyMock struct {
	Gtid  string
	State string
}

func (rmm *rmMock) newRequestBody(gtid string) string {
	pb := rmBodyMock{
		Gtid: gtid,
	}
	b, _ := json.Marshal(&pb)
	return string(b)
}

func (rmm *rmMock) Execute(ctx context.Context, gtid string, url string, bodyStr string, state string, body *rmBodyMock) (string, error) {

	h, ok := rmm.handlers[url]
	if !ok {
		return "", errors.New("not found")
	}

	if body != nil {
		err := json.Unmarshal([]byte(bodyStr), body)
		if err != nil {
			return "", err
		}
	}

	atomic.AddInt32(&h.count, 1)

	select {
	case <-time.After(h.timeout()):
		if h.stores == nil {
			h.stores = make(map[string]*rmBodyMock)
		}
		if body == nil {
			body = &rmBodyMock{
				Gtid:  gtid,
				State: state,
			}
		}
		h.stores[gtid] = body
		return bodyStr, nil
	case <-ctx.Done():
		return "", errors.New("timeout")
	}
}
