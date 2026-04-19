package kube

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

// PortForward opens a local TCP port forwarding to a pod's container port.
// Returns the local address (host:port) and a stop function. The caller must
// call stop when done.
type PortForward struct {
	LocalPort int
	stopCh    chan struct{}
	readyCh   chan struct{}
	fw        *portforward.PortForwarder
}

func (c *Client) ForwardService(ctx context.Context, namespace, service string, remotePort int) (*PortForward, error) {
	svc, err := c.Core.CoreV1().Services(namespace).Get(ctx, service, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get service %s/%s: %w", namespace, service, err)
	}
	pods, err := c.Core.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector(svc.Spec.Selector),
	})
	if err != nil {
		return nil, err
	}
	var target *corev1.Pod
	for i := range pods.Items {
		p := &pods.Items[i]
		if p.Status.Phase == corev1.PodRunning {
			target = p
			break
		}
	}
	if target == nil {
		return nil, fmt.Errorf("no ready pod found for service %s/%s", namespace, service)
	}
	return c.ForwardPod(ctx, target.Namespace, target.Name, remotePort)
}

func (c *Client) ForwardPod(ctx context.Context, namespace, pod string, remotePort int) (*PortForward, error) {
	localPort, err := freePort()
	if err != nil {
		return nil, err
	}

	reqURL, err := podPortForwardURL(c.Config.Host, namespace, pod)
	if err != nil {
		return nil, err
	}
	transport, upgrader, err := spdy.RoundTripperFor(c.Config)
	if err != nil {
		return nil, err
	}
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, reqURL)

	stopCh := make(chan struct{})
	readyCh := make(chan struct{})
	ports := []string{fmt.Sprintf("%d:%d", localPort, remotePort)}
	fw, err := portforward.New(dialer, ports, stopCh, readyCh, io.Discard, io.Discard)
	if err != nil {
		return nil, err
	}
	errCh := make(chan error, 1)
	go func() {
		if err := fw.ForwardPorts(); err != nil {
			errCh <- err
		}
	}()
	select {
	case <-readyCh:
	case err := <-errCh:
		return nil, err
	case <-ctx.Done():
		close(stopCh)
		return nil, ctx.Err()
	}
	return &PortForward{LocalPort: localPort, stopCh: stopCh, readyCh: readyCh, fw: fw}, nil
}

func (p *PortForward) Close() {
	if p == nil || p.stopCh == nil {
		return
	}
	close(p.stopCh)
	p.stopCh = nil
}

func (p *PortForward) URL() string {
	return fmt.Sprintf("http://127.0.0.1:%d", p.LocalPort)
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func podPortForwardURL(host, namespace, pod string) (*url.URL, error) {
	base, err := url.Parse(host)
	if err != nil {
		return nil, err
	}
	base.Path = fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/portforward", namespace, pod)
	return base, nil
}

func labelSelector(m map[string]string) string {
	if len(m) == 0 {
		return ""
	}
	out := ""
	first := true
	for k, v := range m {
		if !first {
			out += ","
		}
		out += fmt.Sprintf("%s=%s", k, v)
		first = false
	}
	return out
}
