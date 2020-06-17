package idgenerator

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

/*
|------------ 40 bit --------|---------- 2 bit ----------|----------- 7 bit ----------|------------ 15 bit ------------|
          milli second                datacenter                       node                        sequence
	 	 34 years                         4                            128                          32768
*/

const (
	AnchorEpoch = int64(1451569374000) // 2015/12/31 21:42:54

	DataCenterBits  = uint(2)
	MaxDataCenterId = -1 ^ (-1 << DataCenterBits)

	NodeIdBits = uint(7)
	MaxNodeId  = -1 ^ (-1 << NodeIdBits)

	SequenceBits      = uint(15)
	NodeIdShift       = SequenceBits
	DataCenterIdShift = SequenceBits + NodeIdBits
	TimestampShift    = SequenceBits + NodeIdBits + DataCenterBits
	MaxSequence       = -1 ^ (-1 << SequenceBits)

	MaxBatchIds = 100000
)

type IdGenerator interface {
	NextId() (int64, error)
	NextIds(num int) ([]int64, error)
}

type snowFlake struct {
	sync.Mutex
	lastTimestamp int64
	nodeId        int64
	datacenterId  int64
	sequence      int64
}

func NewSnowflake(nodeId, datacenterId int64) (IdGenerator, error) {
	idg := &snowFlake{}
	if nodeId > MaxNodeId || nodeId < 0 {
		return nil, fmt.Errorf(fmt.Sprintf("node id should be in range (%d, %d)", 0, MaxNodeId))
	}
	if datacenterId > MaxDataCenterId || datacenterId < 0 {
		return nil, fmt.Errorf(fmt.Sprintf("datacenter id should be in range (%d, %d)", 0, MaxDataCenterId))
	}
	idg.nodeId = nodeId
	idg.datacenterId = datacenterId

	idg.lastTimestamp = nowMilliSecond()

	return idg, nil
}

func nowMilliSecond() int64 {
	return time.Now().UnixNano() / int64(time.Millisecond)
}

func getNextMilliSecond(lastTimestamp int64) int64 {
	millis := nowMilliSecond()

	for millis <= lastTimestamp {
		millis = nowMilliSecond()
	}
	return millis
}

func (id *snowFlake) NextId() (int64, error) {
	id.Lock()
	defer id.Unlock()

	millis := nowMilliSecond()
	if millis < id.lastTimestamp {
		return -1, errors.New("error timestamp")
	}

	if id.lastTimestamp == millis {
		id.sequence = (id.sequence + 1) & MaxSequence

		// overflow : wait next milli second
		if id.sequence == 0 {
			millis = getNextMilliSecond(id.lastTimestamp)
		}
	} else {
		id.sequence = 0
	}

	id.lastTimestamp = millis
	return ((millis - AnchorEpoch) << TimestampShift) | (id.datacenterId << DataCenterIdShift) | (id.nodeId << NodeIdShift) | id.sequence, nil
}

func (id *snowFlake) NextIds(num int) ([]int64, error) {
	if num > MaxBatchIds || num < 0 {
		return nil, errors.New("invalid parameter")
	}
	ids := make([]int64, num)
	id.Lock()
	defer id.Unlock()

	millis := nowMilliSecond()
	if millis < id.lastTimestamp {
		// don't try to getNextMilliSecond
		return nil, errors.New("error timestamp")
	}
	id.lastTimestamp = millis

	for i := 0; i < num; i++ {
		id.sequence = (id.sequence + 1) & MaxSequence
		if id.sequence == 0 {
			millis = getNextMilliSecond(id.lastTimestamp)
			id.lastTimestamp = millis
		}

		ids[i] = ((millis - AnchorEpoch) << TimestampShift) | (id.datacenterId << DataCenterIdShift) | (id.nodeId << NodeIdShift) | id.sequence
	}

	id.lastTimestamp = millis

	return ids, nil
}
