package fake

import (
	"strings"

	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/eip"
	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/executor"
	"github.com/yunify/qingcloud-cloud-controller-manager/test/pkg/e2eutil"
	qcservice "github.com/yunify/qingcloud-sdk-go/service"
)

var _ executor.QingCloudListenerExecutor = &FakeQingCloudLBExecutor{}
var _ executor.QingCloudLoadBalancerExecutor = &FakeQingCloudLBExecutor{}

type FakeQingCloudLBExecutor struct {
	LoadBalancers map[string]*qcservice.LoadBalancer
	ReponseEIPs   map[string]*eip.EIP
	Listeners     map[string]*qcservice.LoadBalancerListener
	Backends      map[string]*qcservice.LoadBalancerBackend
}

func NewFakeQingCloudLBExecutor() *FakeQingCloudLBExecutor {
	f := new(FakeQingCloudLBExecutor)
	f.LoadBalancers = make(map[string]*qcservice.LoadBalancer)
	f.ReponseEIPs = make(map[string]*eip.EIP, 0)
	f.Listeners = make(map[string]*qcservice.LoadBalancerListener)
	f.Backends = make(map[string]*qcservice.LoadBalancerBackend)
	return f
}

func (f *FakeQingCloudLBExecutor) GetLoadBalancerByName(name string) (*qcservice.LoadBalancer, error) {
	for _, lb := range f.LoadBalancers {
		if *lb.LoadBalancerName == name {
			return lb, nil
		}
	}
	return nil, nil
}

func (f *FakeQingCloudLBExecutor) GetLoadBalancerByID(id string) (*qcservice.LoadBalancer, error) {
	return f.LoadBalancers[id], nil
}

func (f *FakeQingCloudLBExecutor) Start(id string) error {
	return nil
}

func (f *FakeQingCloudLBExecutor) Stop(id string) error {
	return nil
}

func (f *FakeQingCloudLBExecutor) Create(input *qcservice.CreateLoadBalancerInput) (*qcservice.LoadBalancer, error) {
	lb := new(qcservice.LoadBalancer)
	id := "lb-" + e2eutil.RandString(8)
	lb.LoadBalancerID = &id
	lb.LoadBalancerName = input.LoadBalancerName
	i, _ := f.GetEIPByID(*input.EIPs[0])
	lb.Cluster = []*qcservice.EIP{i.ToQingCloudEIP()}
	t := *input.LoadBalancerType
	lb.LoadBalancerType = &t
	sg := *input.SecurityGroup
	lb.SecurityGroupID = &sg
	f.LoadBalancers[id] = lb
	return lb, nil
}

func (f *FakeQingCloudLBExecutor) Resize(id string, newtype int) error {
	lb, _ := f.GetLoadBalancerByID(id)
	lb.LoadBalancerType = &newtype
	return nil
}

func (f *FakeQingCloudLBExecutor) Modify(input *qcservice.ModifyLoadBalancerAttributesInput) error {
	return nil
}

func (f *FakeQingCloudLBExecutor) AssociateEip(lbid string, eips ...string) error {
	lb := f.LoadBalancers[lbid]
	for _, eip := range eips {
		i, _ := f.GetEIPByID(eip)
		lb.Cluster = append(lb.Cluster, i.ToQingCloudEIP())
	}
	return nil
}

func (f *FakeQingCloudLBExecutor) DissociateEip(lbid string, eips ...string) error {
	lb := f.LoadBalancers[lbid]
	news := make([]*qcservice.EIP, 0)
	for _, eip := range lb.Cluster {
		index := -1
		for i, v := range eips {
			if v == *eip.EIPID {
				index = i
				break
			}
		}
		if index == -1 {
			news = append(news, eip)
		}
	}
	lb.Cluster = news
	return nil
}

func (f *FakeQingCloudLBExecutor) Confirm(id string) error {
	return nil
}

func (f *FakeQingCloudLBExecutor) Delete(id string) error {
	delete(f.LoadBalancers, id)
	return nil
}

func (f *FakeQingCloudLBExecutor) GetLBAPI() *qcservice.LoadBalancerService {
	return nil
}

func (f *FakeQingCloudLBExecutor) GetListenersOfLB(lbid string, prefix string) ([]*qcservice.LoadBalancerListener, error) {
	result := make([]*qcservice.LoadBalancerListener, 0)
	for _, v := range f.Listeners {
		if *v.LoadBalancerID == lbid {
			if prefix != "" {
				if strings.Contains(*v.LoadBalancerListenerName, prefix) {
					result = append(result, v)
				}
			} else {
				result = append(result, v)
			}
		}
	}
	return result, nil
}

func (f *FakeQingCloudLBExecutor) GetListenerByID(id string) (*qcservice.LoadBalancerListener, error) {
	return f.Listeners[id], nil
}

func (f *FakeQingCloudLBExecutor) GetListenerByName(id, name string) (*qcservice.LoadBalancerListener, error) {
	for _, v := range f.Listeners {
		if *v.LoadBalancerListenerName == name && *v.LoadBalancerID == id {
			return v, nil
		}
	}
	return nil, nil
}

func (f *FakeQingCloudLBExecutor) DeleteListener(lsnid string) error {
	delete(f.Listeners, lsnid)
	ids := []string{}
	for _, b := range f.Backends {
		if *b.LoadBalancerListenerID == lsnid {
			ids = append(ids, *b.LoadBalancerBackendID)
		}
	}
	if len(ids) > 0 {
		f.DeleteBackends(ids...)
	}
	return nil
}

func (f *FakeQingCloudLBExecutor) CreateListener(input *qcservice.AddLoadBalancerListenersInput) (*qcservice.LoadBalancerListener, error) {
	l := input.Listeners[0]
	id := "listener-" + e2eutil.RandString(8)
	l.LoadBalancerListenerID = &id
	l.LoadBalancerID = input.LoadBalancer
	f.Listeners[id] = l
	return l, nil
}

func (f *FakeQingCloudLBExecutor) ModifyListener(id string, balanceMode string) error {
	f.Listeners[id].BalanceMode = &balanceMode
	return nil
}

func (f *FakeQingCloudLBExecutor) DeleteBackends(ids ...string) error {
	for _, b := range ids {
		delete(f.Backends, b)
	}
	return nil
}

func (f *FakeQingCloudLBExecutor) GetBackendsOfListener(lsnid string) ([]*qcservice.LoadBalancerBackend, error) {
	result := make([]*qcservice.LoadBalancerBackend, 0)
	for _, b := range f.Backends {
		if *b.LoadBalancerListenerID == lsnid {
			result = append(result, b)
		}
	}
	return result, nil
}

func (f *FakeQingCloudLBExecutor) GetBackendByID(bid string) (*qcservice.LoadBalancerBackend, error) {
	return f.Backends[bid], nil
}

func (f *FakeQingCloudLBExecutor) CreateBackends(input *qcservice.AddLoadBalancerBackendsInput) error {
	for _, b := range input.Backends {
		b.LoadBalancerListenerID = input.LoadBalancerListener
		id := "backend-" + e2eutil.RandString(8)
		b.LoadBalancerBackendID = &id
		f.Backends[id] = b
	}
	return nil
}

func (f *FakeQingCloudLBExecutor) ModifyBackend(id string, weight int, port int) error {
	f.Backends[id].Weight = &weight
	f.Backends[id].Port = &port
	return nil
}
