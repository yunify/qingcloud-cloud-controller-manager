package fake

import (
	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/eip"
	"github.com/yunify/qingcloud-cloud-controller-manager/test/pkg/e2eutil"
)

var _ eip.EIPHelper = &FakeQingCloudLBExecutor{}

func (f *FakeQingCloudLBExecutor) GetEIPByID(id string) (*eip.EIP, error) {
	return f.ReponseEIPs[id], nil
}

func (f *FakeQingCloudLBExecutor) GetEIPByAddr(addr string) (*eip.EIP, error) {
	for _, eip := range f.ReponseEIPs {
		if eip.Address == addr {
			return eip, nil
		}
	}
	return nil, nil
}

func (f *FakeQingCloudLBExecutor) ReleaseEIP(id string) error {
	delete(f.ReponseEIPs, id)
	return nil
}

func (f *FakeQingCloudLBExecutor) GetAvaliableOrAllocateEIP() (*eip.EIP, error) {
	r, _ := f.GetAvaliableEIPs()
	if len(r) > 0 {
		return r[0], nil
	}
	return f.AllocateEIP()
}

func (f *FakeQingCloudLBExecutor) AllocateEIP() (*eip.EIP, error) {
	ip := new(eip.EIP)
	ip.ID = "eip-" + e2eutil.RandString(8)
	ip.Name = eip.AllocateEIPName
	ip.Address = e2eutil.RandIP()
	ip.Status = "allocate"
	f.ReponseEIPs[ip.ID] = ip
	return ip, nil
}

func (f *FakeQingCloudLBExecutor) GetAvaliableEIPs() ([]*eip.EIP, error) {
	result := make([]*eip.EIP, 0)
	for _, v := range f.ReponseEIPs {
		if v.Status == "avaliable" {
			result = append(result, v)
		}
	}
	return result, nil
}

func (f *FakeQingCloudLBExecutor) AddEIP(id, ip string) *eip.EIP {
	i := &eip.EIP{
		ID:      id,
		Address: ip,
		Status:  "avaliable",
	}
	f.ReponseEIPs[id] = i
	return i
}
