package kube

import (
	"bytes"
	"context"
	"fmt"
	"github.com/mattfenwick/cyclonus/pkg/utils"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
)

type Kubernetes struct {
	ClientSet  *kubernetes.Clientset
	RestConfig *rest.Config
}

func NewKubernetesForContext(context string) (*Kubernetes, error) {
	log.Debugf("instantiating k8s Clientset for context %s", context)
	kubeConfig, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{CurrentContext: context}).ClientConfig()
	if err != nil {
		return nil, errors.Wrapf(err, "unable to build config")
	}
	clientset, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to instantiate Clientset")
	}
	return &Kubernetes{
		ClientSet:  clientset,
		RestConfig: kubeConfig,
	}, nil
}

func (k *Kubernetes) GetNamespace(namespace string) (*v1.Namespace, error) {
	ns, err := k.ClientSet.CoreV1().Namespaces().Get(context.TODO(), namespace, metav1.GetOptions{})
	return ns, errors.Wrapf(err, "unable to get namespace %s", namespace)
}

func (k *Kubernetes) SetNamespaceLabels(namespace string, labels map[string]string) (*v1.Namespace, error) {
	ns, err := k.GetNamespace(namespace)
	if err != nil {
		return nil, err
	}
	ns.Labels = labels
	_, err = k.ClientSet.CoreV1().Namespaces().Update(context.TODO(), ns, metav1.UpdateOptions{})
	return ns, errors.Wrapf(err, "unable to update namespace %s", namespace)
}

func (k *Kubernetes) DeleteNamespace(ns string) error {
	err := k.ClientSet.CoreV1().Namespaces().Delete(context.TODO(), ns, metav1.DeleteOptions{})
	return errors.Wrapf(err, "unable to delete namespace %s", ns)
}

func (k *Kubernetes) CreateOrUpdateNamespace(ns *v1.Namespace) (*v1.Namespace, error) {
	nsr, err := k.ClientSet.CoreV1().Namespaces().Create(context.TODO(), ns, metav1.CreateOptions{})
	if err == nil {
		log.Debugf("created namespace %s", ns)
		return nsr, nil
	}

	log.Debugf("unable to create namespace %s, let's try updating it instead (error: %s)", ns.Name, err)
	nsr, err = k.ClientSet.CoreV1().Namespaces().Update(context.TODO(), ns, metav1.UpdateOptions{})
	return nsr, errors.Wrapf(err, "unable to update namespace %s", ns.Name)
}

func (k *Kubernetes) DeleteAllNetworkPoliciesInNamespace(ns string) error {
	log.Debugf("deleting all network policies in namespace %s", ns)
	netpols, err := k.ClientSet.NetworkingV1().NetworkPolicies(ns).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return errors.Wrapf(err, "unable to list network policies in ns %s", ns)
	}
	for _, np := range netpols.Items {
		log.Debugf("deleting network policy %s/%s", ns, np.Name)
		err = k.DeleteNetworkPolicy(np.Namespace, np.Name)
		if err != nil {
			return err
		}
	}
	return nil
}

func (k *Kubernetes) DeleteAllNetworkPoliciesInNamespaces(nss []string) error {
	for _, ns := range nss {
		err := k.DeleteAllNetworkPoliciesInNamespace(ns)
		if err != nil {
			return err
		}
	}
	return nil
}

func (k *Kubernetes) DeleteNetworkPolicy(ns string, name string) error {
	err := k.ClientSet.NetworkingV1().NetworkPolicies(ns).Delete(context.TODO(), name, metav1.DeleteOptions{})
	return errors.Wrapf(err, "unable to delete network policy %s/%s", ns, name)
}

func (k *Kubernetes) GetNetworkPoliciesInNamespaces(namespaces []string) ([]networkingv1.NetworkPolicy, error) {
	var netpols []networkingv1.NetworkPolicy
	for _, ns := range namespaces {
		podList, err := k.ClientSet.NetworkingV1().NetworkPolicies(ns).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return nil, errors.Wrapf(err, "unable to get netpols in namespace %s", ns)
		}
		netpols = append(netpols, podList.Items...)
	}
	return netpols, nil
}

func (k *Kubernetes) UpdateNetworkPolicy(policy *networkingv1.NetworkPolicy) (*networkingv1.NetworkPolicy, error) {
	log.Debugf("updating network policy %s/%s", policy.Namespace, policy.Name)
	np, err := k.ClientSet.NetworkingV1().NetworkPolicies(policy.Namespace).Update(context.TODO(), policy, metav1.UpdateOptions{})
	return np, errors.Wrapf(err, "unable to update network policy %s/%s", policy.Namespace, policy.Name)
}

func (k *Kubernetes) CreateNetworkPolicy(policy *networkingv1.NetworkPolicy) (*networkingv1.NetworkPolicy, error) {
	log.Debugf("creating network policy %s/%s", policy.Namespace, policy.Name)

	createdPolicy, err := k.ClientSet.NetworkingV1().NetworkPolicies(policy.Namespace).Create(context.TODO(), policy, metav1.CreateOptions{})
	return createdPolicy, errors.Wrapf(err, "unable to create network policy %s/%s", policy.Namespace, policy.Name)
}

func (k *Kubernetes) GetService(namespace string, name string) (*v1.Service, error) {
	service, err := k.ClientSet.CoreV1().Services(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	return service, errors.Wrapf(err, "unable to get service %s/%s", namespace, name)
}

func (k *Kubernetes) CreateService(svc *v1.Service) (*v1.Service, error) {
	ns := svc.Namespace
	log.Debugf("creating service %s/%s", ns, svc.Name)
	createdService, err := k.ClientSet.CoreV1().Services(ns).Create(context.TODO(), svc, metav1.CreateOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "unable to create service %s/%s", ns, svc.Name)
	}
	return createdService, nil
}

func (k *Kubernetes) DeleteService(namespace string, name string) error {
	log.Debugf("deleting service %s/%s", namespace, name)
	err := k.ClientSet.CoreV1().Services(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
	return errors.Wrapf(err, "unable to delete service %s/%s", namespace, name)
}

func (k *Kubernetes) CreateOrUpdateService(svc *v1.Service) (*v1.Service, error) {
	nsr, err := k.ClientSet.CoreV1().Services(svc.Namespace).Create(context.TODO(), svc, metav1.CreateOptions{})
	if err == nil {
		log.Debugf("created service %s/%s", svc.Namespace, svc.Name)
		return nsr, nil
	}

	log.Debugf("unable to create service %s/%s, let's try updating it instead (error: %s)", svc.Namespace, svc.Name, err)
	nsr, err = k.ClientSet.CoreV1().Services(svc.Namespace).Update(context.TODO(), svc, metav1.UpdateOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "unable to update service %s/%s", svc.Namespace, svc.Name)
	}
	return nsr, nil
}

func (k *Kubernetes) CreateServiceIfNotExists(svc *v1.Service) (*v1.Service, error) {
	created, err := k.ClientSet.CoreV1().Services(svc.Namespace).Create(context.TODO(), svc, metav1.CreateOptions{})
	if err == nil {
		return created, nil
	}
	if err.Error() == fmt.Sprintf(`services "%s" already exists`, svc.Name) {
		return nil, nil
	}
	return nil, err
}

func (k *Kubernetes) GetPodsInNamespaces(namespaces []string) ([]v1.Pod, error) {
	var pods []v1.Pod
	for _, ns := range namespaces {
		podList, err := k.ClientSet.CoreV1().Pods(ns).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return nil, errors.Wrapf(err, "unable to get pods in namespace %s", ns)
		}
		pods = append(pods, podList.Items...)
	}
	return pods, nil
}

func (k *Kubernetes) GetPod(namespace string, podName string) (*v1.Pod, error) {
	pod, err := k.ClientSet.CoreV1().Pods(namespace).Get(context.TODO(), podName, metav1.GetOptions{})
	return pod, errors.Wrapf(err, "unable to get pod %s/%s", namespace, podName)
}

func (k *Kubernetes) SetPodLabels(namespace string, podName string, labels map[string]string) (*v1.Pod, error) {
	pod, err := k.GetPod(namespace, podName)
	if err != nil {
		return nil, err
	}
	pod.Labels = labels
	updatedPod, err := k.ClientSet.CoreV1().Pods(namespace).Update(context.TODO(), pod, metav1.UpdateOptions{})
	return updatedPod, errors.Wrapf(err, "unable to update pod %s/%s", namespace, podName)
}

func (k *Kubernetes) CreatePod(pod *v1.Pod) (*v1.Pod, error) {
	ns := pod.Namespace
	log.Debugf("creating pod %s/%s", ns, pod.Name)

	createdPod, err := k.ClientSet.CoreV1().Pods(ns).Create(context.TODO(), pod, metav1.CreateOptions{})
	return createdPod, errors.Wrapf(err, "unable to create pod %s/%s", ns, pod.Name)
}

func (k *Kubernetes) CreatePodIfNotExists(pod *v1.Pod) (*v1.Pod, error) {
	created, err := k.ClientSet.CoreV1().Pods(pod.Namespace).Create(context.TODO(), pod, metav1.CreateOptions{})
	if err == nil {
		return created, nil
	}
	log.Warnf("%+v", err)
	if err.Error() == fmt.Sprintf(`pods "%s" already exists`, pod.Name) {
		return nil, nil
	}
	return nil, errors.Wrapf(err, "unable to create pod %s/%s:\n%s", pod.Namespace, pod.Name, utils.JsonString(pod))
}

func (k *Kubernetes) DeletePod(namespace string, podName string) error {
	log.Debugf("deleting pod %s/%s", namespace, podName)
	err := k.ClientSet.CoreV1().Pods(namespace).Delete(context.TODO(), podName, metav1.DeleteOptions{})
	return errors.Wrapf(err, "unable to delete pod %s/%s", namespace, podName)
}

// ExecuteRemoteCommand executes a remote shell command on the given pod
// returns the output from stdout and stderr
func (k *Kubernetes) ExecuteRemoteCommand(namespace string, pod string, container string, command []string) (string, string, error, error) {
	request := k.ClientSet.
		CoreV1().
		RESTClient().
		Post().
		Namespace(namespace).
		Resource("pods").
		Name(pod).
		SubResource("exec").
		Param("container", container).
		//Timeout(5*time.Second). // TODO this seems to not do anything ... why ?
		VersionedParams(
			&v1.PodExecOptions{
				Container: container,
				Command:   command,
				Stdin:     false,
				Stdout:    true,
				Stderr:    true,
				TTY:       true,
			},
			scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(k.RestConfig, "POST", request.URL())
	if err != nil {
		return "", "", nil, errors.Wrapf(err, "unable to instantiate SPDYExecutor")
	}

	buf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	err = exec.Stream(remotecommand.StreamOptions{
		Stdout: buf,
		Stderr: errBuf,
	})

	out, errOut := buf.String(), errBuf.String()
	return out, errOut, errors.Wrapf(err, "unable to stream command"), nil
}
