package fake

import (
	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/eip"
)

func (f *FakeQingCloudLBExecutor) GetEIPByID(id string) (*eip.EIP, error) {
	for _, eip := range f.ReponseEIPs {
		if eip.ID == id {
			return eip, nil
		}
	}
	return nil, nil
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
	return nil
}

func (f *FakeQingCloudLBExecutor) GetAvaliableOrAllocateEIP() (*eip.EIP, error) {
	return f.ReponseEIPs[0], nil
}

func (f *FakeQingCloudLBExecutor) AllocateEIP() (*eip.EIP, error) {
	return f.ReponseEIPs[0], nil
}

func (f *FakeQingCloudLBExecutor) GetAvaliableEIPs() ([]*eip.EIP, error) {
	return f.ReponseEIPs, nil
}
