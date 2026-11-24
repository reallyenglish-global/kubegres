package kindcluster

import (
	"fmt"
	"net/url"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func namedConfig(data []byte) (*rest.Config, error) {
	raw, err := clientcmd.Load(data)
	if err != nil {
		return nil, fmt.Errorf("invalid Kind kubeconfig")
	}
	const name = "kind-" + clusterName
	ctx := raw.Contexts[name]
	if ctx == nil || ctx.Cluster != name || ctx.AuthInfo != name || raw.Clusters[name] == nil || raw.AuthInfos[name] == nil {
		return nil, fmt.Errorf("missing or inconsistent %q cluster binding", name)
	}
	endpoint, err := url.Parse(raw.Clusters[name].Server)
	if err != nil || endpoint.Scheme != "https" || endpoint.Hostname() == "" {
		return nil, fmt.Errorf("missing or invalid Kind API endpoint")
	}
	return clientcmd.NewNonInteractiveClientConfig(*raw, name, &clientcmd.ConfigOverrides{}, nil).ClientConfig()
}
