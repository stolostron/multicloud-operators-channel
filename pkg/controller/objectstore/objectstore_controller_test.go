// Licensed Materials - Property of IBM
// (c) Copyright IBM Corporation 2016, 2019. All Rights Reserved.
// US Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP  Schedule Contract with IBM Corp.

package objectstore

import (
	"encoding/json"
	"io/ioutil"
	"testing"
	"time"

	"github.com/ghodss/yaml"
	"github.com/golang/glog"
	"github.com/onsi/gomega"
	chnv1alpha1 "github.com/IBM/multicloud-operators-channel/pkg/apis/app/v1alpha1"
	"github.com/IBM/multicloud-operators-channel/pkg/utils"
	dplv1alpha1 "github.com/IBM/multicloud-operators-deployable/pkg/apis/app/v1alpha1"
	"golang.org/x/net/context"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var c client.Client

var objectStoreEndpoint = "http://127.0.0.1:9000"
var objectStoreAccessKeyID = "admin"
var objectStoreSecretAccessKey = "admin111"

var channelNamespace = "ch-qa"

var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: "test-deployable", Namespace: channelNamespace}}
var expectedRequestObj = reconcile.Request{NamespacedName: types.NamespacedName{Name: "from-object-bucket", Namespace: channelNamespace}}

const timeout = time.Second * 5

var dplObj utils.DeployableObject

func TestReconcile(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	// Setup the Manager and Controller.
	// Wrap the Controller Reconcile function so it writes each request to a channel when it is finished.
	mgr, err := manager.New(cfg, manager.Options{})

	g.Expect(err).NotTo(gomega.HaveOccurred())

	c = mgr.GetClient()
	chdesc, err := utils.CreateChannelDescriptor()

	recFn, requests := SetupTestReconcile(newReconciler(mgr, chdesc))
	g.Expect(add(mgr, recFn)).NotTo(gomega.HaveOccurred())

	stopMgr, mgrStopped := StartTestManager(mgr, g)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	awshandler := &utils.AWSHandler{}
	err = awshandler.InitObjectStoreConnection(objectStoreEndpoint, objectStoreAccessKeyID, objectStoreSecretAccessKey)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	// ----- Create Bucket: ch-qa -----
	err = awshandler.Create(channelNamespace)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	// ----- Create Namespace: ch-qa -----
	nsobj := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: channelNamespace,
		},
	}

	clientset, err := kubernetes.NewForConfig(cfg)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	_, err = clientset.CoreV1().Namespaces().Create(nsobj)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	nsobjFetched := &v1.Namespace{}
	nsobjFetched, err = clientset.CoreV1().Namespaces().Get(channelNamespace, metav1.GetOptions{})
	g.Expect(err).NotTo(gomega.HaveOccurred())
	glog.V(10).Infof("nameSpace created : %+v", nsobjFetched)

	// ----- Create new Channel instance -----
	var chnobj chnv1alpha1.Channel
	var chndata []byte
	// current directoy is deployable-objectstore/pkg/controller/objectstore/
	chndata, err = ioutil.ReadFile("../../../hack/test/test_channel.yaml")
	g.Expect(err).NotTo(gomega.HaveOccurred())

	err = yaml.Unmarshal(chndata, &chnobj)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	err = c.Create(context.TODO(), &chnobj)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	defer c.Delete(context.TODO(), &chnobj)

	// ----- Create new Secret instance -----
	var secobj v1.Secret
	var secdata []byte
	// current directoy is deployable-objectstore/pkg/controller/objectstore/
	secdata, err = ioutil.ReadFile("../../../hack/test/test_secret.yaml")
	g.Expect(err).NotTo(gomega.HaveOccurred())

	err = yaml.Unmarshal(secdata, &secobj)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	err = c.Create(context.TODO(), &secobj)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	defer c.Delete(context.TODO(), &secobj)

	// ----- Upload some templates to objectstore to test channel sync -----
	var tplNoHosting = "tplNoHosting"
	var tplWithHosting = "tplWithHosting"
	// current directoy is deployable-objectstore/pkg/controller/objectstore/
	tpldata, err := ioutil.ReadFile("../../../hack/test/test_template.yaml")
	g.Expect(err).NotTo(gomega.HaveOccurred())

	// Upload template WITHOUT AnnotationHosting

	dplObj = utils.DeployableObject{
		Name:    tplNoHosting,
		Content: tpldata,
	}

	err = awshandler.Put(channelNamespace, dplObj)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	// Upload template WITH AnnotationHosting
	tpl := &unstructured.Unstructured{}
	err = yaml.Unmarshal(tpldata, tpl)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	tplannotation := make(map[string]string)
	hosting := types.NamespacedName{Name: "test", Namespace: channelNamespace}.String()
	tplannotation[dplv1alpha1.AnnotationHosting] = hosting
	tpl.SetAnnotations(tplannotation)
	tplb, err := yaml.Marshal(tpl)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	dplObj = utils.DeployableObject{
		Name:    tplWithHosting,
		Content: tplb,
	}
	err = awshandler.Put(channelNamespace, dplObj)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	// ---------- Create ----------
	// Create new Deployable instance
	var dplobj dplv1alpha1.Deployable
	var dpldata []byte
	// current directoy is deployable-objectstore/pkg/controller/objectstore/
	dpldata, err = ioutil.ReadFile("../../../hack/test/test_deployable.yaml")
	g.Expect(err).NotTo(gomega.HaveOccurred())

	err = yaml.Unmarshal(dpldata, &dplobj)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	err = c.Create(context.TODO(), &dplobj)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	// Expect the Reconcile to be triggered
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))
	// Expect that channel description is configred properly
	chdescription, ok := chdesc.Get(chnobj.Name)
	g.Expect(ok).To(gomega.BeTrue())
	g.Expect(chdescription.Bucket).To(gomega.Equal(chnobj.Namespace))
	err = chdescription.ObjectStore.Exists(chnobj.Namespace)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	// Expect that the template is uploaded to ObjectStore with proper annotations
	objb, err := chdescription.ObjectStore.Get(chdescription.Bucket, dplobj.Name)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	objtpl := &unstructured.Unstructured{}
	err = yaml.Unmarshal(objb.Content, objtpl)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	tplannotations := objtpl.GetAnnotations()
	// Check AnnotationHosting is added to template
	expectedhosting := types.NamespacedName{Name: dplobj.GetName(), Namespace: dplobj.GetNamespace()}.String()
	actualhosting, ok := tplannotations[dplv1alpha1.AnnotationHosting]
	g.Expect(ok).To(gomega.BeTrue())
	g.Expect(actualhosting).To(gomega.Equal(expectedhosting))
	// Check deployable annotations are carried with template to the objectstore
	actuallocal, ok := tplannotations[dplv1alpha1.AnnotationLocal]
	g.Expect(ok).To(gomega.BeTrue())
	g.Expect(actuallocal).To(gomega.Equal("false"))
	actualversion, ok := tplannotations[dplv1alpha1.AnnotationDeployableVersion]
	g.Expect(ok).To(gomega.BeTrue())
	g.Expect(actualversion).To(gomega.Equal("1.1.0"))
	// Check deployable labels are carried with template to the objectstore
	tpllabels := objtpl.GetLabels()
	actuallabel, ok := tpllabels["deployable-label"]
	g.Expect(ok).To(gomega.BeTrue())
	g.Expect(actuallabel).To(gomega.Equal("passed-in"))

	// Check that channel sync deleted old template WITH AnnotationHosting from objectstore
	_, err = chdescription.ObjectStore.Get(chdescription.Bucket, tplWithHosting)
	g.Expect(err.Error()).To(gomega.HavePrefix("NoSuchKey: The specified key does not exist"))

	// Check that channel sync didn't delete template WITHOUT AnnotationHosting from objectstore
	_, err = chdescription.ObjectStore.Get(chdescription.Bucket, tplNoHosting)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	// Manually delete it to cleanup
	err = chdescription.ObjectStore.Delete(chdescription.Bucket, tplNoHosting)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	// ---------- Update ----------
	// Update deployable
	dpltpl := &unstructured.Unstructured{}
	err = json.Unmarshal(dplobj.Spec.Template.Raw, dpltpl)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	lbls := make(map[string]string)
	lbls["test-label"] = "test-value"
	dpltpl.SetLabels(lbls)
	dplobj.Spec.Template.Raw, err = json.Marshal(dpltpl)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	err = c.Update(context.TODO(), &dplobj)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	// Expect the Reconcile to be triggered
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))
	// Expect that the template is updated on objectstore
	objb, err = chdescription.ObjectStore.Get(chdescription.Bucket, dplobj.Name)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	objtpl = &unstructured.Unstructured{}
	err = yaml.Unmarshal(objb.Content, objtpl)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	tplannotations = objtpl.GetAnnotations()
	// Check AnnotationHosting is added to template
	expectedhosting = types.NamespacedName{Name: dplobj.GetName(), Namespace: dplobj.GetNamespace()}.String()
	actualhosting, ok = tplannotations[dplv1alpha1.AnnotationHosting]
	g.Expect(ok).To(gomega.BeTrue())
	g.Expect(actualhosting).To(gomega.Equal(expectedhosting))
	// Check deployable annotations are carried with template to the objectstore
	actuallocal, ok = tplannotations[dplv1alpha1.AnnotationLocal]
	g.Expect(ok).To(gomega.BeTrue())
	g.Expect(actuallocal).To(gomega.Equal("false"))
	actualversion, ok = tplannotations[dplv1alpha1.AnnotationDeployableVersion]
	g.Expect(ok).To(gomega.BeTrue())
	g.Expect(actualversion).To(gomega.Equal("1.1.0"))
	// Check deployable labels are carried with template to the objectstore
	tpllabels = objtpl.GetLabels()
	actuallabel1, ok := tpllabels["deployable-label"]
	g.Expect(ok).To(gomega.BeTrue())
	g.Expect(actuallabel1).To(gomega.Equal("passed-in"))
	// Check the actual update (new label added to template itself) is applied
	actuallabel2, ok := tpllabels["test-label"]
	g.Expect(ok).To(gomega.BeTrue())
	g.Expect(actuallabel2).To(gomega.Equal("test-value"))

	// ---------- Delete ----------
	// Delete deployable
	err = c.Delete(context.TODO(), &dplobj)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	// Expect the Reconcile to be triggered
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))
	// Expect that the template is deleted from objectstore
	_, err = chdescription.ObjectStore.Get(chdescription.Bucket, dplobj.Name)
	g.Expect(err.Error()).To(gomega.HavePrefix("NoSuchKey: The specified key does not exist"))

	// ---------- Create ----------
	// Create a deployable for a template from objectstore
	var tplbyte []byte
	// current directoy is deployable-objectstore/pkg/controller/objectstore/
	tplbyte, err = ioutil.ReadFile("../../../hack/test/test_template.yaml")
	g.Expect(err).NotTo(gomega.HaveOccurred())
	template := &unstructured.Unstructured{}
	err = yaml.Unmarshal(tplbyte, template)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	tplannots := make(map[string]string)
	tplannots[dplv1alpha1.AnnotationExternalSource] = "http://127.0.0.1:9000/ch-qa"
	template.SetAnnotations(tplannots)

	dpl := &dplv1alpha1.Deployable{}
	dpl.Name = template.GetName()
	dpl.Namespace = chnobj.Namespace
	dpl.Spec.Template = &runtime.RawExtension{}
	dpl.Spec.Template.Raw, err = json.Marshal(template)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	err = c.Create(context.TODO(), dpl)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	// Expect the Reconcile to be triggered
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequestObj)))
	// Expect that the template is not uploaded to objectstore
	// because the template has AnnotationExternalSource set
	_, err = chdescription.ObjectStore.Get(chdescription.Bucket, dpl.Name)
	g.Expect(err.Error()).To(gomega.HavePrefix("NoSuchKey: The specified key does not exist"))

	// Manually put the template on objectstore for the following delete test

	dplObj = utils.DeployableObject{
		Name:    dpl.Name,
		Content: tpldata,
	}

	err = chdescription.ObjectStore.Put(chdescription.Bucket, dplObj)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	// ---------- Delete ----------
	// Delete the deployable
	err = c.Delete(context.TODO(), dpl)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	// Expect the Reconcile to be triggered
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequestObj)))
	// Expect that deployable is deleted
	dpllist := &dplv1alpha1.DeployableList{}
	err = c.List(context.TODO(), &client.ListOptions{Namespace: chnobj.Namespace}, dpllist)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(len(dpllist.Items)).To(gomega.Equal(0))
	// Expect that the template is not deleted from objectstore
	// because the template doesn't have AnnotationHosting set
	_, err = chdescription.ObjectStore.Get(chdescription.Bucket, dpl.Name)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	// Manually delete the template from objectstore
	err = chdescription.ObjectStore.Delete(chdescription.Bucket, dpl.Name)
	g.Expect(err).NotTo(gomega.HaveOccurred())
}
