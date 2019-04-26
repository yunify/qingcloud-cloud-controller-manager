package qingcloud

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/klog"
)

var _ cloudprovider.Zones = &QingCloud{}

func (qc *QingCloud) Zones() (cloudprovider.Zones, bool) {
	return qc, true
}
func (qc *QingCloud) GetZone(ctx context.Context) (cloudprovider.Zone, error) {
	klog.V(4).Infof("GetZone() called, current zone is %v", qc.zone)

	return cloudprovider.Zone{Region: qc.zone}, nil
}

// GetZoneByNodeName implements Zones.GetZoneByNodeName
// This is particularly useful in external cloud providers where the kubelet
// does not initialize node data.
func (qc *QingCloud) GetZoneByNodeName(ctx context.Context, nodeName types.NodeName) (cloudprovider.Zone, error) {
	klog.V(4).Infof("GetZoneByNodeName() called, current zone is %v, and return zone directly as temporary solution", qc.zone)
	return cloudprovider.Zone{Region: qc.zone}, nil
}

// GetZoneByProviderID implements Zones.GetZoneByProviderID
// This is particularly useful in external cloud providers where the kubelet
// does not initialize node data.
func (qc *QingCloud) GetZoneByProviderID(ctx context.Context, providerID string) (cloudprovider.Zone, error) {
	klog.V(4).Infof("GetZoneByProviderID() called, current zone is %v, and return zone directly as temporary solution", qc.zone)
	return cloudprovider.Zone{Region: qc.zone}, nil
}
