package k8s

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/paceval/paceval/examples_sources/NodeJS_examples/k8s/pacevalAPIService/pkg/data"
	"github.com/rs/zerolog/log"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"os"
)

var (
	gvk = schema.GroupVersionKind{
		Group:   "paceval.com",
		Version: "v1alpha1",
		Kind:    "PacevalComputationObject",
	}
	gvr = schema.GroupVersionResource{
		Group:    "paceval.com",
		Version:  "v1alpha1",
		Resource: "pacevalcomputationobjects",
	}
)

type Manager struct {
	client dynamic.Interface
}

func NewK8sManager() Manager {
	clientset, err := getClientSet()

	if err != nil {
		log.Fatal().Msgf("fatal error: %s", err)
		panic("fail to get the clientset")
	}

	return Manager{client: clientset}

}

func getClientSet() (dynamic.Interface, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		// If not running inside a Kubernetes cluster, use the default kubeconfig file.
		kubeconfig := os.Getenv("HOME") + "/.kube/config"
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, err
		}
	}

	client, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return client, nil
}

func (r Manager) CreateComputation(params *data.ParameterSet) (string, error) {
	log.Info().Msgf("create computation on parameters %v", params)
	id := uuid.New()
	instanceName := fmt.Sprintf("paceval-computation-%s", id.String())
	obj := unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	obj.SetName(instanceName)
	obj.SetNamespace(data.DEFAULTNAMESPACE)

	spec := make(map[string]interface{})
	spec["Internal"] = params.Interval
	spec["NumOfVars"] = params.NumOfVariables
	spec["Vars"] = params.Variables
	spec["functionId"] = id.String()
	spec["functionStr"] = params.FunctionStr
	obj.Object["spec"] = spec

	_, err := r.client.Resource(gvr).Namespace(data.DEFAULTNAMESPACE).Create(context.TODO(), &obj, metav1.CreateOptions{})

	if err != nil {
		log.Error().Msgf("Error when creating computation: %s", err)
		return "", err
	}

	return id.String(), nil
}

func (r Manager) GetEndpoint(id string) (string, error) {
	instanceName := fmt.Sprintf("paceval-computation-%s", id)
	instance, err := r.client.Resource(gvr).Namespace(data.DEFAULTNAMESPACE).Get(context.TODO(), instanceName, metav1.GetOptions{})

	if err != nil {
		return "", err
	}

	ready, err := r.checkServiceReady(instance)

	if err != nil {
		return "", err
	}

	if !ready {
		return "", errors.New("computation is not ready")
	}

	endpoint, _, err := unstructured.NestedString(instance.Object, "status", "endpoint")

	if err != nil {
		return "", err
	}

	err = r.updateLastActiveTimeStamp(instance)

	if err != nil {
		return "", err
	}

	return endpoint, nil

}

func (r Manager) updateLastActiveTimeStamp(instance *unstructured.Unstructured) error {
	instance.Object["status"].(map[string]interface{})["lastActiveTime"] = metav1.Now()
	_, err := r.client.Resource(gvr).Namespace(data.DEFAULTNAMESPACE).UpdateStatus(context.TODO(), &unstructured.Unstructured{Object: instance.Object}, metav1.UpdateOptions{})
	return err
}

func (r Manager) checkServiceReady(instance *unstructured.Unstructured) (bool, error) {
	ready, _, err := unstructured.NestedString(instance.Object, "status", "ready")

	if err != nil {
		return false, err
	}

	if ready != string(v1.ConditionTrue) {
		return false, nil
	}

	return true, nil
}

func (r Manager) DeleteComputation(id string) error {
	instanceName := fmt.Sprintf("paceval-computation-%s", id)
	return r.client.Resource(gvr).Namespace(data.DEFAULTNAMESPACE).Delete(context.TODO(), instanceName, metav1.DeleteOptions{})
}