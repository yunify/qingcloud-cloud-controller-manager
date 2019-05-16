package pool

import (
	"sync"

	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/eip"
	qcservice "github.com/yunify/qingcloud-sdk-go/service"
	"k8s.io/klog"
)

const (
	PoolLBPrefixName = "KUBESPHERE_LB_POOL_NOT_USE_THIS_PREFIX_"
)

type LBPool struct {
	lock sync.RWMutex
	data []*LBInfo
}

type LBInfo struct {
	ID  string
	EIP *eip.EIP
}

func NewLBPool() *LBPool {
	return &LBPool{
		data: make([]*LBInfo, 0),
	}
}

func (l *LBPool) AddLBToPool(lb *LBInfo) error {
	l.lock.Lock()
	defer l.lock.Unlock()
	klog.V(3).Infof("Add LB %s to pool", lb.ID)
	l.data = append(l.data, lb)
	return nil
}

func (l *LBPool) PopLB() *LBInfo {
	l.lock.Lock()
	defer l.lock.Unlock()
	klog.V(3).Infoln("Pop a lb to use")
	if len(l.data) == 0 {
		return nil
	}
	lb := l.data[len(l.data)-1]
	l.data = l.data[:len(l.data)-1]
	return lb
}

func (l *LBPool) Length() int {
	return len(l.data)
}

func QclbToPoolElement(lb *qcservice.LoadBalancer) *LBInfo {
	return &LBInfo{
		ID:  *lb.LoadBalancerID,
		EIP: eip.ConvertQingCloudEIP(lb.Cluster[0]),
	}
}
