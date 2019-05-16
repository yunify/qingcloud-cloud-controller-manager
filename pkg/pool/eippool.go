package pool

import (
	"sync"

	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/eip"
	"k8s.io/klog"
)

type EIPPool struct {
	lock sync.RWMutex
	data []*eip.EIP
}

func NewEIPPool() *EIPPool {
	return &EIPPool{
		data: make([]*eip.EIP, 0),
	}
}

func (e *EIPPool) AddEIP(eip *eip.EIP) {
	e.lock.Lock()
	defer e.lock.Unlock()
	klog.V(3).Infof("Adding eip %s to pool", eip.Address)
	e.data = append(e.data, eip)
}

func (e *EIPPool) PopEIP() *eip.EIP {
	e.lock.Lock()
	defer e.lock.Unlock()
	klog.V(3).Infoln("Pop a eip to use")
	if len(e.data) == 0 {
		return nil
	}
	eip := e.data[len(e.data)-1]
	e.data = e.data[:len(e.data)-1]
	return eip
}

func (e *EIPPool) Length() int {
	return len(e.data)
}

func (e *EIPPool) PopReleasableEIP() *eip.EIP {
	for index, ip := range e.data {
		if ip.Source == eip.AllocateOnly {
			if index < (len(e.data) - 1) {
				tmp := e.data[index+1:]
				e.data = append(e.data[:index], tmp...)
			} else {
				e.data = e.data[:index]
			}
			return ip
		}
	}
	return nil
}
