package qingcloud

// See https://docs.qingcloud.com/api/volume/index.html

import (
	"fmt"
	"strings"
	qcservice "github.com/yunify/qingcloud-sdk-go/service"
	qcclient "github.com/yunify/qingcloud-sdk-go/client"
	"github.com/golang/glog"
)

// DefaultMaxQingCloudVolumes is the limit for volumes attached to an instance.
// TODO: No clear description from qingcloud document
//const DefaultMaxQingCloudVolumes = 6

// VolumeOptions specifies capacity and type for a volume.
// See https://docs.qingcloud.com/api/volume/create_volumes.html
type VolumeOptions struct {
	CapacityGB int // minimum 10GiB, maximum 500GiB, must be a multiple of 10x
	VolumeType int // only can be 0, 1, 2, 3
	VolumeName string
}

// Volumes is an interface for managing cloud-provisioned volumes
type Volumes interface {
	// Attach the disk to the specified instance
	// Returns the device (e.g. /dev/sdb) where we attached the volume
	// It checks if volume is already attached to node and succeeds in that case.
	AttachVolume(volumeID string, instanceID string) (string, error)

	// Detach the disk from the specified instance
	DetachVolume(volumeID string, instanceID string) error

	// Create a volume with the specified options
	CreateVolume(volumeOptions *VolumeOptions) (volumeID string, err error)

	// Delete the specified volume
	// Returns true if the volume was deleted
	// If the was not found, returns (false, nil)
	DeleteVolume(volumeID string) (bool, error)

	// Check if the volume is already attached to the instance
	VolumeIsAttached(volumeID string, instanceID string) (bool, error)

	// Check if a list of volumes are attached to the node with the specified NodeName
	DisksAreAttached(volumeIDs []string, instanceID string) (map[string]bool, error)
}

// AttachVolume implements Volumes.AttachVolume
func (qc *QingCloud) AttachVolume(volumeID string, instanceID string) (string, error) {
	glog.V(4).Infof("AttachVolume(%v,%v) called", volumeID, instanceID)

	attached, err := qc.VolumeIsAttached(volumeID, instanceID)
	if err != nil {
		return "", err
	}

	if !attached {
		output, err := qc.volumeService.AttachVolumes(&qcservice.AttachVolumesInput{
			Volumes:[]*string{ &volumeID},
			Instance: &instanceID,
		})
		if err != nil {
			return "", err
		}
		jobID := *output.JobID
		err = qcclient.WaitJob(qc.jobService, jobID, operationWaitTimeout, waitInterval)
		if err != nil {
			return "", err
		}
	}

	output, err := qc.volumeService.DescribeVolumes(&qcservice.DescribeVolumesInput{
		Volumes: []*string{&volumeID},
	})
	if err != nil {
		return "", err
	}
	if len(output.VolumeSet) == 0 {
		return "", fmt.Errorf("volume '%v' miss after attach it", volumeID)
	}

	dev := output.VolumeSet[0].Instance.Device
	if dev == nil || *dev == "" {
		return "", fmt.Errorf("the device of volume '%v' is empty", volumeID)
	}

	return *dev, nil
}

// DetachVolume implements Volumes.DetachVolume
func (qc *QingCloud) DetachVolume(volumeID string, instanceID string) error {
	glog.V(4).Infof("DetachVolume(%v,%v) called", volumeID, instanceID)

	output, err := qc.volumeService.DetachVolumes(&qcservice.DetachVolumesInput{
		Volumes: []*string{&volumeID},
		Instance: &instanceID,
	})
	if err != nil {
		return err
	}
	jobID := *output.JobID
	err = qcclient.WaitJob(qc.jobService, jobID, operationWaitTimeout, waitInterval)
	return err
}

// CreateVolume implements Volumes.CreateVolume
func (qc *QingCloud) CreateVolume(volumeOptions *VolumeOptions) (string, error) {
	glog.V(4).Infof("CreateVolume(%v) called", volumeOptions)

	output, err := qc.volumeService.CreateVolumes(&qcservice.CreateVolumesInput{
		VolumeName: &volumeOptions.VolumeName,
		Size:       &volumeOptions.CapacityGB,
		VolumeType: &volumeOptions.VolumeType,
	})
	if err != nil {
		return "", err
	}
	jobID := *output.JobID
	qcclient.WaitJob(qc.jobService, jobID, operationWaitTimeout, waitInterval)
	return *output.Volumes[0], nil
}

// DeleteVolume implements Volumes.DeleteVolume
func (qc *QingCloud) DeleteVolume(volumeID string) (bool, error) {
	glog.V(4).Infof("DeleteVolume(%v) called", volumeID)

	output, err := qc.volumeService.DeleteVolumes(&qcservice.DeleteVolumesInput{
		Volumes: []*string{&volumeID},
	})
	if err != nil {
		if strings.Index(err.Error(), "already been deleted") >= 0 {
			return false, nil
		}
		return false, err
	}

	jobID := *output.JobID
	qcclient.WaitJob(qc.jobService, jobID, operationWaitTimeout, waitInterval)

	return true, nil
}

// VolumeIsAttached implements Volumes.VolumeIsAttached
func (qc *QingCloud) VolumeIsAttached(volumeID string, instanceID string) (bool, error) {
	glog.V(4).Infof("VolumeIsAttached(%v,%v) called", volumeID, instanceID)

	output, err := qc.volumeService.DescribeVolumes(&qcservice.DescribeVolumesInput{
		Volumes: []*string{&volumeID},
	})
	if err != nil {
		return false, err
	}
	if len(output.VolumeSet) == 0 {
		return false, nil
	}

	return *output.VolumeSet[0].Instance.InstanceID == instanceID, nil
}

func (qc *QingCloud)  DisksAreAttached(volumeIDs []string, instanceID string) (map[string]bool, error){
	glog.V(4).Infof("DisksAreAttached(%v,%v) called", volumeIDs, instanceID)

	attached := make(map[string]bool)
	for _, volumeID := range volumeIDs {
		attached[volumeID] = false
	}
	output, err := qc.volumeService.DescribeVolumes(&qcservice.DescribeVolumesInput{
		Volumes: qcservice.StringSlice(volumeIDs),
	})
	if err != nil {
		return nil, err
	}
	for _, volume := range output.VolumeSet {
		if *volume.Instance.InstanceID == instanceID {
			attached[*volume.VolumeID] = true
		}
	}
	return attached, nil
}