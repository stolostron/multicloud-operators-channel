module github.com/open-cluster-management/multicloud-operators-channel

go 1.13

require (
	github.com/Masterminds/semver v1.5.0 // indirect
	github.com/aws/aws-sdk-go-v2 v0.18.0
	github.com/ghodss/yaml v1.0.1-0.20190212211648-25d852aebe32
	github.com/go-logr/logr v0.1.0
	github.com/go-logr/zapr v0.1.1
	github.com/go-openapi/spec v0.19.4
	github.com/google/go-cmp v0.4.0
	github.com/onsi/ginkgo v1.12.0
	github.com/onsi/gomega v1.9.0
	github.com/open-cluster-management/api v0.0.0-20200623215229-19a96fed707a
	github.com/open-cluster-management/multicloud-operators-deployable v0.0.0-20200625020633-0d458afa0fc2
	github.com/open-cluster-management/multicloud-operators-placementrule v0.0.0-20200624201731-5e0510647fa9
	github.com/operator-framework/operator-sdk v0.18.0
	github.com/pkg/errors v0.9.1
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.5.1
	go.uber.org/zap v1.14.1
	golang.org/x/net v0.0.0-20200421231249-e086a090c8fd
	gopkg.in/yaml.v2 v2.2.8
	k8s.io/api v0.18.3
	k8s.io/apiextensions-apiserver v0.18.2
	k8s.io/apimachinery v0.18.3
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/helm v2.16.3+incompatible
	k8s.io/klog v1.0.0
	k8s.io/kube-openapi v0.0.0-20200410145947-61e04a5be9a6
	sigs.k8s.io/controller-runtime v0.6.0
)

replace (
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v13.3.2+incompatible // Required by OLM
	k8s.io/client-go => k8s.io/client-go v0.18.2
)

replace golang.org/x/net => github.com/golang/net v0.0.0-20191209160850-c0dbc17a3553
