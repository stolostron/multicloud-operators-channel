module github.com/stolostron/multicloud-operators-channel

go 1.15

require (
	github.com/Masterminds/semver v1.5.0 // indirect
	github.com/aws/aws-sdk-go-v2 v0.18.0
	github.com/cyphar/filepath-securejoin v0.2.2 // indirect
	github.com/emicklei/go-restful v2.11.1+incompatible // indirect
	github.com/ghodss/yaml v1.0.1-0.20190212211648-25d852aebe32
	github.com/go-logr/logr v0.3.0
	github.com/go-logr/zapr v0.3.0
	github.com/go-openapi/spec v0.19.5
	github.com/gobwas/glob v0.2.3 // indirect
	github.com/google/go-cmp v0.5.2
	github.com/onsi/ginkgo v1.12.1
	github.com/onsi/gomega v1.10.1
	github.com/open-cluster-management/api v0.0.0-20201007180356-41d07eee4294
	github.com/open-cluster-management/multicloud-operators-deployable v1.2.2-2-20201130-7bc3c
	github.com/open-cluster-management/multicloud-operators-placementrule v1.2.2-2-20201130-98cfd
	github.com/pkg/errors v0.9.1
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.6.1
	go.uber.org/zap v1.14.1
	golang.org/x/net v0.0.0-20211112202133-69e39bad7dc2
	golang.org/x/tools v0.0.0-20200713235242-6acd2ab80ede // indirect
	gopkg.in/yaml.v3 v3.0.0-20200615113413-eeeca48fe776 // indirect
	k8s.io/api v0.20.0
	k8s.io/apiextensions-apiserver v0.19.3
	k8s.io/apimachinery v0.20.0
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/helm v2.17.0+incompatible
	k8s.io/klog v1.0.0
	k8s.io/kube-openapi v0.0.0-20201113171705-d219536bb9fd
	sigs.k8s.io/controller-runtime v0.6.3
)

replace (
	github.com/open-cluster-management/api => open-cluster-management.io/api v0.0.0-20201007180356-41d07eee4294
	github.com/open-cluster-management/multicloud-operators-deployable => github.com/stolostron/multicloud-operators-deployable v1.2.2-2-20201130-7bc3c
	github.com/open-cluster-management/multicloud-operators-placementrule => github.com/stolostron/multicloud-operators-placementrule v1.2.2-2-20201130-98cfd
	k8s.io/api => k8s.io/api v0.19.3
	k8s.io/client-go => k8s.io/client-go v0.19.3
)
