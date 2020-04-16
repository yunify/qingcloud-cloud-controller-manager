package eip

import (
	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/errors"
	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/executor"
	qcclient "github.com/yunify/qingcloud-sdk-go/client"
	qcservice "github.com/yunify/qingcloud-sdk-go/service"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog"
)

const (
	DefaultBandWidth = 2

	EIPStatusAvailable = "available"
	AllocateEIPName    = "k8s_lb_allocate_eip"
	ResourceNameEIP    = "EIP"
)

type NewEIPHelperOfQingCloudOption struct {
	JobAPI *qcservice.JobService
	EIPAPI *qcservice.EIPService
	TagAPI *qcservice.TagService
	TagIDs []string
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
		return nil, errors.NewCommonServerError(ResourceNameEIP, id, "GetEIPByID", err.Error())
	}
	if *output.TotalCount < 1 {
		return nil, errors.NewResourceNotFoundError(ResourceNameEIP, id)
	}
	return q.ConvertQingCloudEIP(output.EIPSet[0]), nil
}

func (q *qingcloudEIPHelper) GetEIPByAddr(addr string) (*EIP, error) {
	output, err := q.EIPAPI.DescribeEIPs(&qcservice.DescribeEIPsInput{
		SearchWord: &addr,
	})
	if err != nil {
		return nil, errors.NewCommonServerError(ResourceNameEIP, addr, "GetEIPByAddr", err.Error())
	}
	for _, item := range output.EIPSet {
		if *item.EIPAddr == addr {
			return q.ConvertQingCloudEIP(item), nil
		}
	}
	return nil, errors.NewResourceNotFoundError(ResourceNameEIP, addr)
}

func (q *qingcloudEIPHelper) ReleaseEIP(id string) error {
	output, err := q.EIPAPI.ReleaseEIPs(&qcservice.ReleaseEIPsInput{
		EIPs: []*string{&id},
	})
	if err != nil {
		return errors.NewCommonServerError(ResourceNameEIP, id, "ReleaseEIP", err.Error())
	}

	if *output.RetCode != 0 {
		return errors.NewCommonServerError(ResourceNameEIP, id, "ReleaseEIP", *output.Message)
	}
	return qcclient.WaitJob(q.JobAPI, *output.JobID, operationWaitTimeout, waitInterval)
}

func (q *qingcloudEIPHelper) GetAvaliableOrAllocateEIP() (*EIP, error) {
	eips, err := q.GetAvaliableEIPs()
	if err != nil {
		if errors.IsResourceNotFound(err) {
			return q.AllocateEIP()
		}
		return nil, err
	}
	return eips[0], nil
}

func (q *qingcloudEIPHelper) AllocateEIP() (*EIP, error) {
	output, err := q.EIPAPI.AllocateEIPs(&qcservice.AllocateEIPsInput{
		Bandwidth: qcservice.Int(DefaultBandWidth),
		EIPName:   qcservice.String(AllocateEIPName),
	})
	if err != nil {
		return nil, errors.NewCommonServerError(ResourceNameEIP, "", "AllocateEIP", err.Error())
	}

	if *output.RetCode != 0 {
		return nil, errors.NewCommonServerError(ResourceNameEIP, "", "AllocateEIP", *output.Message)
	}
	eip, err := q.waitEIPStatus(*output.EIPs[0], EIPStatusAvailable)
	if err != nil {
		return nil, errors.NewCommonServerError(ResourceNameEIP, *output.EIPs[0], "waitEIPStatus", err.Error())
	}
	if len(q.TagIDs) > 0 {
		var eips []string
		for _, eip := range output.EIPs {
			if eip != nil {
				eips = append(eips, *eip)
			}
		}
		err = executor.AttachTagsToResources(q.TagAPI, q.TagIDs, eips, "eip")
		if err != nil {
			klog.Errorf("Failed to add tags to Eip %v, err: %s", eips, err.Error())
		} else {
			klog.Infof("Add tag %s to eip %v done", q.TagIDs, eips)
		}
	}
	return eip, nil
}

func (q *qingcloudEIPHelper) GetAvaliableEIPs() ([]*EIP, error) {
	output, err := q.EIPAPI.DescribeEIPs(&qcservice.DescribeEIPsInput{
		Owner:  &q.UserID,
		Status: []*string{qcservice.String(EIPStatusAvailable)},
	})

	if err != nil {
		return nil, errors.NewCommonServerError(ResourceNameEIP, "", "GetAvaliableEIPs", err.Error())
	}

	if *output.RetCode != 0 {
		return nil, errors.NewCommonServerError(ResourceNameEIP, "", "GetAvaliableEIPs", *output.Message)
	}
	result := make([]*EIP, 0)
	for _, item := range output.EIPSet {
		if *item.AssociateMode == 0 {
			result = append(result, q.ConvertQingCloudEIP(item))
		}
	}
	if len(result) == 0 {
		return nil, errors.NewResourceNotFoundError(ResourceNameEIP, "available eips")
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
