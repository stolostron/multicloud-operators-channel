module github.com/open-cluster-management/multicloud-operators-channel

go 1.13

require (
	github.com/aws/aws-sdk-go-v2 v0.18.0
	github.com/ghodss/yaml v1.0.1-0.20190212211648-25d852aebe32
	github.com/go-logr/logr v0.1.0
	github.com/go-logr/zapr v0.1.1
	github.com/go-openapi/spec v0.19.4
	github.com/google/go-cmp v0.4.0
	github.com/onsi/ginkgo v1.12.0
	github.com/onsi/gomega v1.9.0
	github.com/open-cluster-management/multicloud-operators-deployable v0.0.0-20200513210312-92eab4faeb2b
	github.com/open-cluster-management/multicloud-operators-placementrule v1.0.0-2020-05-12-22-38-29.0.20200513204034-766ba50d9664
	github.com/operator-framework/operator-sdk v0.17.0
	github.com/pkg/errors v0.9.1
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.4.0
	go.uber.org/zap v1.14.1
	golang.org/x/net v0.0.0-20200226121028-0de0cce0169b
	gopkg.in/yaml.v2 v2.2.8
	k8s.io/api v0.17.4
	k8s.io/apiextensions-apiserver v0.17.4
	k8s.io/apimachinery v0.17.4
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/cluster-registry v0.0.6
	k8s.io/helm v2.16.3+incompatible
	k8s.io/klog v1.0.0
	k8s.io/kube-openapi v0.0.0-20191107075043-30be4d16710a
	sigs.k8s.io/controller-runtime v0.5.2
)

replace (
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v13.3.2+incompatible // Required by OLM
	k8s.io/client-go => k8s.io/client-go v0.17.4 // Required by prometheus-operator
)

replace golang.org/x/net => github.com/golang/net v0.0.0-20191209160850-c0dbc17a3553
