package eip

import (
	"fmt"

	qcservice "github.com/yunify/qingcloud-sdk-go/service"
)

var (
	ErrorEIPNotFound = fmt.Errorf("Cound not find the eip")
)

type EIPAllocateSource string

const (
	ManualSet                 EIPAllocateSource = "manual"
	UseAvailableOrAllocateOne EIPAllocateSource = "auto"
	UseAvailableOnly          EIPAllocateSource = "use-available"
	AllocateOnly              EIPAllocateSource = "allocate"
	Unknow                    EIPAllocateSource = "unknow"
)

type EIP struct {
	Name        string
	ID          string
	Address     string
	Status      string
	Bandwidth   int
	BillingMode string
	Source      EIPAllocateSource
}

type EIPHelper interface {
	GetEIPByID(id string) (*EIP, error)
	GetEIPByAddr(addr string) (*EIP, error)
	ReleaseEIP(id string) error
	GetAvaliableOrAllocateEIP() (*EIP, error)
	AllocateEIP() (*EIP, error)
	GetAvaliableEIPs() ([]*EIP, error)
}

func (e *EIP) ToQingCloudEIP() *qcservice.EIP {
	return &qcservice.EIP{
		EIPID:   &e.ID,
		EIPAddr: &e.Address,
		EIPName: &e.Name,
		Status:  &e.Status,
	}
}

func StringToEIPAllocateType(str string) EIPAllocateSource {
	switch str {
	case string(AllocateOnly):
		return AllocateOnly
	case string(UseAvailableOnly):
		return UseAvailableOnly
	case string(UseAvailableOrAllocateOne):
		return UseAvailableOrAllocateOne
	default:
		return ManualSet
	}
}

func (e *EIP) setAllocateSourceByName() {
	if e.Name == AllocateEIPName {
		e.Source = AllocateOnly
	}
	e.Source = ManualSet
}
