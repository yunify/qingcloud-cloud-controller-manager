package qingcloud

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

const (
	ListenerTimeoutMin = 10
	ListenerTimeoutMax = 86400
)

var (
	ListenerSceneValueSets = []int{0, 1, 11}
)

// data format like this: port1:conf,port2:conf,port3:conf
// Example:
//
//	1)healthycheckmethod: "80:tcp,443:tcp"
//	2)healthycheckoption: "80:10|5|2|5,443:10|5|2|5"
//	3)balancemode: "80:roundrobin,443:leastconn,8080:source"
//	4)cert: "443:sc-77oko7zj,80:sc-77oko7zj"
//	5)protocol: "443:https,80:http"

func parseLsnAnnotaionData(data string) (map[int]string, error) {
	methods := strings.Split(data, ",")
	rst := make(map[int]string, len(methods))
	for _, method := range methods {
		if method == "" {
			continue
		}
		m := strings.Split(method, ":")
		if len(m) != 2 {
			return nil, fmt.Errorf("wrong format: (%s)", data)
		}
		port, err := strconv.Atoi(m[0])
		if err != nil {
			return nil, fmt.Errorf("wrong format: (%s)", data)
		}
		rst[port] = m[1]
	}
	return rst, nil
}

// data format like this: port1:intConf,port2:intConf,port3:intConf
// Example:

// 1)timeout: "443:10,80:20"
// 2)scene: "443:1,80:0"
// 3)forwardfor: "443:1"
// 4)listeneroption: "443:4"

func parseAnnotationIntoIntIntMap(annotationConfValue string) (map[int]int, error) {
	result := make(map[int]int)
	items := strings.Split(annotationConfValue, ",")
	if len(items) == 0 {
		return nil, nil
	}
	for _, item := range items {
		parts := strings.Split(item, ":")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid format")
		}
		k, err1 := strconv.Atoi(parts[0])
		v, err2 := strconv.Atoi(parts[1])
		if err1 != nil || err2 != nil {
			return nil, fmt.Errorf("invalid value")
		}
		result[k] = v

	}
	return result, nil
}

func validListenerStringConfig(annotationKey, annotationValue string) error {
	_, err := parseLsnAnnotaionData(annotationValue)
	if err != nil {
		return fmt.Errorf("invalid config for nanotation %s: %v", annotationKey, err)
	}

	return nil
}

func validListenerIntConfig(annotationKey, annotationValue string) error {
	_, err := parseAnnotationIntoIntIntMap(annotationValue)
	if err != nil {
		return fmt.Errorf("invalid config for nanotation %s: %v", annotationKey, err)
	}

	return nil
}

func validListenerTimeout(annotationTimeoutConf string) error {
	result, err := parseAnnotationIntoIntIntMap(annotationTimeoutConf)
	if err != nil {
		return fmt.Errorf("invalid timeout config for nanotation %s: %v ", ServiceAnnotationListenerTimeout, err)
	}

	// the value must in range 10 ～ 86400
	for _, value := range result {
		if value < ListenerTimeoutMin || value > ListenerTimeoutMax {
			return fmt.Errorf("invalid timeout config value for nanotation %s, please spec a timeout value in range (%d, %d)",
				ServiceAnnotationListenerTimeout, ListenerTimeoutMin, ListenerTimeoutMax)
		}
	}

	return nil
}

func validListenerScene(annotationSceneConf string) error {
	result, err := parseAnnotationIntoIntIntMap(annotationSceneConf)
	if err != nil {
		return fmt.Errorf("invalid scene config for nanotation %s: %v ", ServiceAnnotationListenerScene, err)
	}

	// the value must in  [0,1,11]
	for _, value := range result {
		if !slices.Contains(ListenerSceneValueSets, value) {
			return fmt.Errorf("invalid listener scene config value for nanotation %s, please spec a timeout value in set %v",
				ServiceAnnotationListenerTimeout, ListenerSceneValueSets)
		}
	}

	return nil
}
