package fake

import (
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	corev1 "k8s.io/client-go/listers/core/v1"
)

var _ corev1.NodeLister = &FakeNodeLister{}

type FakeNodeLister struct {
}

func (f *FakeNodeLister) List(selector labels.Selector) (ret []*v1.Node, err error) {
	return nil, nil
}

func (f *FakeNodeLister) Get(name string) (*v1.Node, error) {
	node := &v1.Node{}
	node.Name = name
	return node, nil
}

func (f *FakeNodeLister) ListWithPredicate(predicate corev1.NodeConditionPredicate) ([]*v1.Node, error) {
	return nil, nil
}
