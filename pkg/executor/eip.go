package executor

import (
	"fmt"
	"strings"

	"github.com/davecgh/go-spew/spew"
	qcclient "github.com/yunify/qingcloud-sdk-go/client"
	qcservice "github.com/yunify/qingcloud-sdk-go/service"
	"github.com/yunify/qingcloud-sdk-go/utils"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"

	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/apis"
)

const (
	DefaultBandWidth   = 2
	EIPStatusAvailable = "available"
	AllocateEIPName    = "k8s_lb_allocate_eip"
)

func (q *QingCloudClient) getEIPByID(id string) (*qcservice.EIP, error) {
	output, err := q.EIPService.DescribeEIPs(&qcservice.DescribeEIPsInput{
		EIPs: []*string{&id},
	})
	if err != nil || *output.RetCode != 0 {
		klog.V(4).Infof("failed to get eip by id %s, err=%s, output=%s", id, spew.Sdump(err), spew.Sdump(output))
		if err != nil {
			return nil, fmt.Errorf("failed to get eip by id %s, err=%v", id, err)
		}
		return nil, fmt.Errorf("failed to get eip by id %s, code=%d, msg=%s", id, *output.RetCode, *output.Message)
	}

	return output.EIPSet[0], nil
}

func (q *QingCloudClient) ReleaseEIP(ids []*string) error {
	if len(ids) <= 0 {
		return nil
	}

	var (
		err    error
		output *qcservice.ReleaseEIPsOutput
	)

	input := &qcservice.ReleaseEIPsInput{
		EIPs: ids,
	}

	err = utils.WaitForSpecificOrError(func() (bool, error) {
		output, err = q.EIPService.ReleaseEIPs(input)
		//QingCloud Error: Code (1400), Message (PermissionDenied, resource [eip-5ywkioa5] lease info not ready yet, please try later)
		if (err != nil && !strings.Contains(err.Error(), "QingCloud Error: Code (1400)")) ||
			(output != nil && *output.RetCode != 0 && *output.RetCode != 1400) {
			klog.V(4).Infof("failed to release eip, err=%s, input=%s, output=%s", spew.Sdump(err), spew.Sdump(input), spew.Sdump(output))
			return false, fmt.Errorf("failed to release eip, err=%v", err)
		} else if err == nil && *output.RetCode == 0 {
			return true, nil
		}

		return false, nil
	}, operationWaitTimeout, eipWaitInterval)
	if err != nil {
		return err
	}

	return qcclient.WaitJob(q.jobService, *output.JobID, operationWaitTimeout, eipWaitInterval)
}

func fillEIPDefaultFileds(input *qcservice.AllocateEIPsInput) {
	if input.EIPName == nil {
		input.EIPName = qcservice.String(AllocateEIPName)
	}

	if input.Bandwidth == nil {
		input.Bandwidth = qcservice.Int(DefaultBandWidth)
	}
}

func (q *QingCloudClient) AllocateEIP(eip *apis.EIP) (*apis.EIP, error) {
	if eip == nil {
		eip = &apis.EIP{
			Spec: apis.EIPSpec{},
		}
	}

	input := &qcservice.AllocateEIPsInput{
		Bandwidth: eip.Spec.Bandwidth,
		EIPName:   eip.Spec.EIPName,
	}
	fillEIPDefaultFileds(input)

	output, err := q.EIPService.AllocateEIPs(input)
	if err != nil || *output.RetCode != 0 {
		klog.V(4).Infof("failed to allocate eip, err=%s, input=%s, output=%s", spew.Sdump(err), spew.Sdump(input), spew.Sdump(output))
		if err != nil {
			return nil, fmt.Errorf("failed to allocate eip, err=%v", err)
		}
		return nil, fmt.Errorf("failed to allocate eip, code=%d, msg=%s", *output.RetCode, *output.Message)
	}

	id := output.EIPs[0]
	err = q.waitEIPStatus(*id, EIPStatusAvailable)
	if err != nil {
		return nil, fmt.Errorf("eip %s still not available", *id)
	}

	eip.Status.EIPID = id
	err = q.attachTagsToResources([]*string{id}, EIPTagResourceType)
	if err != nil {
		klog.Errorf("Failed to add tags to Eip %v, err: %s", id, err.Error())
	}

	return eip, nil
}

func convertEIP(eip *qcservice.EIP) *apis.EIP {
	return &apis.EIP{
		Spec: apis.EIPSpec{
			EIPName:   eip.EIPName,
			Bandwidth: eip.Bandwidth,
		},
		Status: apis.EIPStatus{
			EIPID: eip.EIPID,
		},
	}
}

func (q *QingCloudClient) GetAvaliableEIPs() ([]*apis.EIP, error) {
	input := &qcservice.DescribeEIPsInput{
		Owner:  &q.Config.UserID,
		Status: []*string{qcservice.String(EIPStatusAvailable)},
	}
	output, err := q.EIPService.DescribeEIPs(input)

	if err != nil || *output.RetCode != 0 {
		klog.V(4).Infof("failed to get avaliable eips, err=%s, input=%s, output=%s", spew.Sdump(err), spew.Sdump(input), spew.Sdump(output))
		if err != nil {
			return nil, fmt.Errorf("failed to get avaliable eips, err=%v", err)
		}
		return nil, fmt.Errorf("failed to get avaliable eips, code=%d, msg=%s", *output.RetCode, *output.Message)
	}

	result := make([]*apis.EIP, 0)
	for _, item := range output.EIPSet {
		if *item.AssociateMode == 0 {
			result = append(result, convertEIP(item))
		}
	}

	return result, nil
}

func (q *QingCloudClient) waitEIPStatus(eipid, status string) error {
	err := wait.Poll(eipWaitInterval, operationWaitTimeout, func() (bool, error) {
		eip, err := q.getEIPByID(eipid)
		if err != nil {
			return false, err
		}
		if *eip.Status == status {
			return true, nil
		}
		klog.V(3).Infof("Waiting for eip %s to be %s", eipid, status)
		return false, nil
	})
	return err
}
