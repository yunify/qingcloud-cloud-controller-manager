package eip

import (
	"fmt"

	qcclient "github.com/yunify/qingcloud-sdk-go/client"
	qcservice "github.com/yunify/qingcloud-sdk-go/service"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog"
)

const (
	DefaultBandWidth = 2

	EIPStatusAvailable = "available"
	AllocateEIPName    = "k8s_lb_allocate_eip"
)

type NewEIPHelperOfQingCloudOption struct {
	JobAPI *qcservice.JobService
	EIPAPI *qcservice.EIPService
	UserID string
}

// NewEIPHelperOfQingCloud create a eip helper of qingcloud
func NewEIPHelperOfQingCloud(opt NewEIPHelperOfQingCloudOption) EIPHelper {
	return &qingcloudEIPHelper{NewEIPHelperOfQingCloudOption: opt}
}

type qingcloudEIPHelper struct {
	NewEIPHelperOfQingCloudOption
}

func (q *qingcloudEIPHelper) ConvertQingCloudEIP(eip *qcservice.EIP) *EIP {
	if eip == nil {
		return nil
	}
	result := new(EIP)
	if eip.EIPAddr != nil {
		result.Address = *eip.EIPAddr
	}
	if eip.EIPID != nil {
		result.ID = *eip.EIPID
	}
	if eip.EIPName != nil {
		result.Name = *eip.EIPName
	}
	if eip.Status != nil {
		result.Status = *eip.Status
	}
	if eip.Bandwidth != nil {
		result.Bandwidth = *eip.Bandwidth
	}
	if eip.BillingMode != nil {
		result.BillingMode = *eip.BillingMode
	}
	return result
}

func (q *qingcloudEIPHelper) GetEIPByID(id string) (*EIP, error) {
	output, err := q.EIPAPI.DescribeEIPs(&qcservice.DescribeEIPsInput{
		EIPs: []*string{&id},
	})
	if err != nil {
		klog.Errorf("Failed to get eip %s from qingcloud", id)
		return nil, err
	}
	if *output.TotalCount < 1 {
		return nil, ErrorEIPNotFound
	}
	return q.ConvertQingCloudEIP(output.EIPSet[0]), nil
}

func (q *qingcloudEIPHelper) GetEIPByAddr(addr string) (*EIP, error) {
	output, err := q.EIPAPI.DescribeEIPs(&qcservice.DescribeEIPsInput{
		SearchWord: &addr,
	})
	if err != nil {
		klog.Errorf("Failed to get eip %s from qingcloud by addr", addr)
		return nil, err
	}
	for _, item := range output.EIPSet {
		if *item.EIPAddr == addr {
			return q.ConvertQingCloudEIP(item), nil
		}
	}
	return nil, ErrorEIPNotFound
}

func (q *qingcloudEIPHelper) ReleaseEIP(id string) error {
	output, err := q.EIPAPI.ReleaseEIPs(&qcservice.ReleaseEIPsInput{
		EIPs: []*string{&id},
	})
	if err != nil {
		klog.Errorf("Failed to call release eip in qingcloud to release %s", id)
		return err
	}

	if *output.RetCode != 0 {
		err := fmt.Errorf("Fail to release eip %s of because of '%s'", id, *output.Message)
		return err
	}
	return qcclient.WaitJob(q.JobAPI, *output.JobID, operationWaitTimeout, waitInterval)
}

func (q *qingcloudEIPHelper) GetAvaliableOrAllocateEIP() (*EIP, error) {
	eips, err := q.GetAvaliableEIPs()
	if err != nil {
		if err != ErrorEIPNotFound {
			return nil, err
		}
	}
	if len(eips) > 0 {
		return eips[0], nil
	}
	return q.AllocateEIP()
}

func (q *qingcloudEIPHelper) AllocateEIP() (*EIP, error) {
	output, err := q.EIPAPI.AllocateEIPs(&qcservice.AllocateEIPsInput{
		Bandwidth: qcservice.Int(DefaultBandWidth),
		EIPName:   qcservice.String(AllocateEIPName),
	})
	if err != nil {
		klog.Errorln("Failed to call allocate eip in qingcloud")
		return nil, err
	}

	if *output.RetCode != 0 {
		err := fmt.Errorf("Fail to allocate eip because of '%s'", *output.Message)
		return nil, err
	}
	return q.waitEIPStatus(*output.EIPs[0], EIPStatusAvailable)
}

func (q *qingcloudEIPHelper) GetAvaliableEIPs() ([]*EIP, error) {
	output, err := q.EIPAPI.DescribeEIPs(&qcservice.DescribeEIPsInput{
		Owner:  &q.UserID,
		Status: []*string{qcservice.String(EIPStatusAvailable)},
	})

	if err != nil {
		klog.Errorln("Failed to call get available eips in qingcloud to release")
		return nil, err
	}

	if *output.RetCode != 0 {
		err := fmt.Errorf("Fail to get available eips of because of '%s'", *output.Message)
		return nil, err
	}
	if *output.TotalCount == 0 {
		return nil, ErrorEIPNotFound
	}
	result := make([]*EIP, 0)
	for _, item := range output.EIPSet {
		if *item.AssociateMode == 0 {
			result = append(result, q.ConvertQingCloudEIP(item))
		}
	}
	return result, nil
}

func (q *qingcloudEIPHelper) waitEIPStatus(eipid, status string) (*EIP, error) {
	result := new(EIP)
	err := wait.Poll(waitInterval, operationWaitTimeout, func() (bool, error) {
		eip, err := q.GetEIPByID(eipid)
		if err != nil {
			return false, err
		}
		if eip.Status == status {
			result = eip
			return true, nil
		}
		klog.V(3).Infof("Waiting for eip %s to be %s", eipid, status)
		return false, nil
	})
	return result, err
}
