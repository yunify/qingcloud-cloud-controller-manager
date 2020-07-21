package qingcloud

import (
	"encoding/json"
	"fmt"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/klog"
)

type servicePatcher struct {
	kclient corev1.CoreV1Interface
	base    *v1.Service
	updated *v1.Service
}

func newServicePatcher(kclient corev1.CoreV1Interface, base *v1.Service) servicePatcher {
	return servicePatcher{
		kclient: kclient,
		base:    base.DeepCopy(),
		updated: base,
	}
}

func (sp *servicePatcher) serviceName() string {
	return fmt.Sprintf("%s/%s", sp.base.Namespace, sp.base.Name)
}

func (sp *servicePatcher) Patch() {
	var err = sp.patch()
	if err != nil {
		klog.Infof("Failed to patch service %s %+v, error: %+v", sp.serviceName(), sp.updated.Status, err)
	} else {
		klog.Infof("Patch service %s %+v success", sp.serviceName(), sp.updated.Status)
	}
}

func (sp *servicePatcher) patch() error {
	// Reset spec to make sure only patch for Status or ObjectMeta.
	sp.updated.Spec = sp.base.Spec

	patchBytes, err := getPatchBytes(sp.base, sp.updated)
	if err != nil {
		return err
	}

	_, err = sp.kclient.Services(sp.base.Namespace).Patch(sp.base.Name, types.StrategicMergePatchType, patchBytes, "status")
	return err
}

func getPatchBytes(oldSvc, newSvc *v1.Service) ([]byte, error) {
	oldData, err := json.Marshal(oldSvc)
	if err != nil {
		return nil, fmt.Errorf("failed to Marshal oldData for svc %s/%s: %v", oldSvc.Namespace, oldSvc.Name, err)
	}

	newData, err := json.Marshal(newSvc)
	if err != nil {
		return nil, fmt.Errorf("failed to Marshal newData for svc %s/%s: %v", newSvc.Namespace, newSvc.Name, err)
	}

	patchBytes, err := strategicpatch.CreateTwoWayMergePatch(oldData, newData, v1.Service{})
	if err != nil {
		return nil, fmt.Errorf("failed to CreateTwoWayMergePatch for svc %s/%s: %v", oldSvc.Namespace, oldSvc.Name, err)
	}
	return patchBytes, nil

}
