package manifest_test

import (
	"net/http"

	bdm "code.cloudfoundry.org/cf-operator/pkg/bosh/manifest"
	"code.cloudfoundry.org/cf-operator/pkg/bosh/manifest/fakes"
	bdc "code.cloudfoundry.org/cf-operator/pkg/kube/apis/boshdeployment/v1alpha1"
	"github.com/onsi/gomega/ghttp"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeClient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Resolver", func() {
	var (
		replaceOpsStr string
		removeOpsStr  string
		opaqueOpsStr  string
		urlOpsStr     string

		validManifestPath string
		validOpsPath      string
		invalidOpsPath    string

		resolver         *bdm.Resolver
		client           client.Client
		interpolator     *fakes.FakeInterpolator
		remoteFileServer *ghttp.Server
		expectedManifest *bdm.Manifest
	)

	BeforeEach(func() {
		validManifestPath = "/valid-manifest.yml"
		validOpsPath = "/valid-ops.yml"
		invalidOpsPath = "/invalid-ops.yml"

		replaceOpsStr = `
- type: replace
  path: /instance_groups/name=component1?/instances
  value: 2
`
		removeOpsStr = `
- type: remove
  path: /instance_groups/name=component2?
`
		opaqueOpsStr = `---
- type: replace
  path: /instance_groups/name=component1?/instances
  value: 3
`

		urlOpsStr = `---
- type: replace
  path: /instance_groups/name=component1?/instances
  value: 4`

		client = fakeClient.NewFakeClient(
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "base-manifest",
					Namespace: "default",
				},
				Data: map[string]string{bdc.ManifestSpecName: `---
instance_groups:
  - name: component1
    instances: 1
  - name: component2
    instances: 2
`},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "",
					Namespace: "default",
				},
				Data: map[string][]byte{bdc.ManifestSpecName: []byte(`---
instance_groups:
  - name: component3
    instances: 1
  - name: component4
    instances: 2
`)},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "opaque-manifest",
					Namespace: "default",
				},
				Data: map[string][]byte{bdc.ManifestSpecName: []byte(`---
instance_groups:
  - name: component3
    instances: 1
  - name: component4
    instances: 2
`)},
			},
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "manifest-with-vars",
					Namespace: "default",
				},
				Data: map[string]string{bdc.ManifestSpecName: `---
name: foo
instance_groups:
  - name: component1
    instances: 1
  - name: component2
    instances: 2
    properties:
      password: ((foo-pass.password))
variables:
  - name: foo-pass
    type: password
  - name: router_ca
    type: certificate
    options:
      is_ca: true
      common_name: ((system_domain))
`},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo-deployment.var-system-domain",
					Namespace: "default",
				},
				Data: map[string][]byte{"value": []byte("example.com")},
			},
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "replace-ops",
					Namespace: "default",
				},
				Data: map[string]string{bdc.OpsSpecName: replaceOpsStr},
			},
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "remove-ops",
					Namespace: "default",
				},
				Data: map[string]string{bdc.OpsSpecName: removeOpsStr},
			},
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-ref",
					Namespace: "default",
				},
				Data: map[string]string{},
			},
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-yaml",
					Namespace: "default",
				},
				Data: map[string]string{bdc.ManifestSpecName: "!yaml"},
			},
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-ops",
					Namespace: "default",
				},
				Data: map[string]string{bdc.OpsSpecName: `
- type: invalid-ops
   path: /name
   value: new-deployment
`},
			},
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "missing-key",
					Namespace: "default",
				},
				Data: map[string]string{bdc.OpsSpecName: `
- type: replace
   path: /missing_key
   value: desired_value
`},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "opaque-ops",
					Namespace: "default",
				},
				Data: map[string][]byte{bdc.OpsSpecName: []byte(opaqueOpsStr)},
			},
		)

		remoteFileServer = ghttp.NewServer()
		remoteFileServer.AllowUnhandledRequests = true

		remoteFileServer.RouteToHandler("GET", validManifestPath, ghttp.RespondWith(http.StatusOK, `---
instance_groups:
  - name: component5
    instances: 1`))
		remoteFileServer.RouteToHandler("GET", validOpsPath, ghttp.RespondWith(http.StatusOK, urlOpsStr))
		remoteFileServer.RouteToHandler("GET", invalidOpsPath, ghttp.RespondWith(http.StatusOK, `---
- type: invalid-type
  path: /key
  value: values`))

		interpolator = &fakes.FakeInterpolator{}
		newInterpolatorFunc := func() bdm.Interpolator {
			return interpolator
		}
		resolver = bdm.NewResolver(client, newInterpolatorFunc)
	})

	Describe("ResolveCRD", func() {
		It("works for valid CRs by using config map", func() {
			deployment := &bdc.BOSHDeployment{
				Spec: bdc.BOSHDeploymentSpec{
					Manifest: bdc.Manifest{
						Type: bdc.ConfigMapType,
						Ref:  "base-manifest",
					},
				},
			}
			expectedManifest = &bdm.Manifest{
				InstanceGroups: []*bdm.InstanceGroup{
					{
						Name:      "component1",
						Instances: 1,
					},
					{
						Name:      "component2",
						Instances: 2,
					},
				},
			}

			manifest, err := resolver.WithOpsManifest(deployment, "default")

			Expect(err).ToNot(HaveOccurred())
			Expect(manifest).ToNot(Equal(nil))
			Expect(len(manifest.InstanceGroups)).To(Equal(2))
			Expect(manifest).To(Equal(expectedManifest))
		})

		It("works for valid CRs by using secret", func() {
			deployment := &bdc.BOSHDeployment{
				Spec: bdc.BOSHDeploymentSpec{
					Manifest: bdc.Manifest{
						Type: bdc.SecretType,
						Ref:  "opaque-manifest",
					},
				},
			}
			expectedManifest = &bdm.Manifest{
				InstanceGroups: []*bdm.InstanceGroup{
					{
						Name:      "component3",
						Instances: 1,
					},
					{
						Name:      "component4",
						Instances: 2,
					},
				},
			}

			manifest, err := resolver.WithOpsManifest(deployment, "default")

			Expect(err).ToNot(HaveOccurred())
			Expect(manifest).ToNot(Equal(nil))
			Expect(len(manifest.InstanceGroups)).To(Equal(2))
			Expect(manifest).To(Equal(expectedManifest))
		})

		It("works for valid CRs by using URL", func() {
			deployment := &bdc.BOSHDeployment{
				Spec: bdc.BOSHDeploymentSpec{
					Manifest: bdc.Manifest{
						Type: bdc.URLType,
						Ref:  remoteFileServer.URL() + validManifestPath,
					},
				},
			}
			expectedManifest = &bdm.Manifest{
				InstanceGroups: []*bdm.InstanceGroup{
					{
						Name:      "component5",
						Instances: 1,
					},
				},
			}

			manifest, err := resolver.WithOpsManifest(deployment, "default")

			Expect(err).ToNot(HaveOccurred())
			Expect(manifest).ToNot(Equal(nil))
			Expect(len(manifest.InstanceGroups)).To(Equal(1))
			Expect(manifest).To(Equal(expectedManifest))
		})

		It("works for valid CRs containing one ops", func() {
			interpolator.InterpolateReturns([]byte(`---
instance_groups:
  - name: component1
    instances: 2
  - name: component2
    instances: 2
`), nil)

			deployment := &bdc.BOSHDeployment{
				Spec: bdc.BOSHDeploymentSpec{
					Manifest: bdc.Manifest{
						Type: bdc.ConfigMapType,
						Ref:  "base-manifest",
					},
					Ops: []bdc.Ops{
						{
							Type: bdc.ConfigMapType,
							Ref:  "replace-ops",
						},
					},
				},
			}
			expectedManifest = &bdm.Manifest{
				InstanceGroups: []*bdm.InstanceGroup{
					{
						Name:      "component1",
						Instances: 2,
					},
					{
						Name:      "component2",
						Instances: 2,
					},
				},
			}

			manifest, err := resolver.WithOpsManifest(deployment, "default")

			Expect(err).ToNot(HaveOccurred())
			Expect(manifest).ToNot(Equal(nil))
			Expect(len(manifest.InstanceGroups)).To(Equal(2))
			Expect(manifest).To(Equal(expectedManifest))

			Expect(interpolator.BuildOpsCallCount()).To(Equal(1))
			opsBytes := interpolator.BuildOpsArgsForCall(0)
			Expect(string(opsBytes)).To(Equal(replaceOpsStr))
		})

		It("works for valid CRs containing multi ops", func() {
			interpolator.InterpolateReturns([]byte(`---
instance_groups:
  - name: component1
    instances: 4
`), nil)

			deployment := &bdc.BOSHDeployment{
				Spec: bdc.BOSHDeploymentSpec{
					Manifest: bdc.Manifest{
						Type: bdc.ConfigMapType,
						Ref:  "base-manifest",
					},
					Ops: []bdc.Ops{
						{
							Type: bdc.ConfigMapType,
							Ref:  "replace-ops",
						},
						{
							Type: bdc.SecretType,
							Ref:  "opaque-ops",
						},
						{
							Type: bdc.URLType,
							Ref:  remoteFileServer.URL() + validOpsPath,
						},
						{
							Type: bdc.ConfigMapType,
							Ref:  "remove-ops",
						},
					},
				},
			}
			expectedManifest = &bdm.Manifest{
				InstanceGroups: []*bdm.InstanceGroup{
					{
						Name:      "component1",
						Instances: 4,
					},
				},
			}

			manifest, err := resolver.WithOpsManifest(deployment, "default")

			Expect(err).ToNot(HaveOccurred())
			Expect(manifest).ToNot(Equal(nil))
			Expect(len(manifest.InstanceGroups)).To(Equal(1))
			Expect(manifest).To(Equal(expectedManifest))

			Expect(interpolator.BuildOpsCallCount()).To(Equal(4))
			opsBytes := interpolator.BuildOpsArgsForCall(0)
			Expect(string(opsBytes)).To(Equal(replaceOpsStr))
			opsBytes = interpolator.BuildOpsArgsForCall(1)
			Expect(string(opsBytes)).To(Equal(opaqueOpsStr))
			opsBytes = interpolator.BuildOpsArgsForCall(2)
			Expect(string(opsBytes)).To(Equal(urlOpsStr))
			opsBytes = interpolator.BuildOpsArgsForCall(3)
			Expect(string(opsBytes)).To(Equal(removeOpsStr))
		})

		It("throws an error if the manifest can not be found", func() {
			deployment := &bdc.BOSHDeployment{
				Spec: bdc.BOSHDeploymentSpec{
					Manifest: bdc.Manifest{
						Type: bdc.ConfigMapType,
						Ref:  "not-existing",
					},
				},
			}
			_, err := resolver.WithOpsManifest(deployment, "default")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to retrieve manifest"))
		})

		It("throws an error if the CR is empty", func() {
			deployment := &bdc.BOSHDeployment{
				Spec: bdc.BOSHDeploymentSpec{
					Manifest: bdc.Manifest{
						Type: bdc.ConfigMapType,
						Ref:  "empty-ref",
					},
				},
			}
			_, err := resolver.WithOpsManifest(deployment, "default")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("doesn't contain key manifest"))
		})

		It("throws an error on invalid yaml", func() {
			deployment := &bdc.BOSHDeployment{
				Spec: bdc.BOSHDeploymentSpec{
					Manifest: bdc.Manifest{
						Type: bdc.ConfigMapType,
						Ref:  "invalid-yaml",
					},
				},
			}
			_, err := resolver.WithOpsManifest(deployment, "default")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("yaml: unmarshal errors"))
		})

		It("throws an error if containing unsupported manifest type", func() {
			interpolator.InterpolateReturns(nil, errors.New("fake-error"))
			deployment := &bdc.BOSHDeployment{
				Spec: bdc.BOSHDeploymentSpec{
					Manifest: bdc.Manifest{
						Type: "unsupported_type",
						Ref:  "base-manifest",
					},
				},
			}
			_, err := resolver.WithOpsManifest(deployment, "default")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unrecognized manifest ref type"))
		})

		It("throws an error if ops configMap can not be found", func() {
			deployment := &bdc.BOSHDeployment{
				Spec: bdc.BOSHDeploymentSpec{
					Manifest: bdc.Manifest{
						Type: bdc.ConfigMapType,
						Ref:  "base-manifest",
					},
					Ops: []bdc.Ops{
						{
							Type: bdc.ConfigMapType,
							Ref:  "not-existing",
						},
					},
				},
			}
			_, err := resolver.WithOpsManifest(deployment, "default")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to retrieve ops from configmap"))
		})

		It("throws an error if ops configMap is empty", func() {
			deployment := &bdc.BOSHDeployment{
				Spec: bdc.BOSHDeploymentSpec{
					Manifest: bdc.Manifest{
						Type: bdc.ConfigMapType,
						Ref:  "base-manifest",
					},
					Ops: []bdc.Ops{
						{
							Type: bdc.ConfigMapType,
							Ref:  "empty-ref",
						},
					},
				},
			}
			_, err := resolver.WithOpsManifest(deployment, "default")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("doesn't contain key ops"))
		})

		It("throws an error if build invalid ops", func() {
			interpolator.BuildOpsReturns(errors.New("fake-error"))

			deployment := &bdc.BOSHDeployment{
				Spec: bdc.BOSHDeploymentSpec{
					Manifest: bdc.Manifest{
						Type: bdc.ConfigMapType,
						Ref:  "base-manifest",
					},
					Ops: []bdc.Ops{
						{
							Type: bdc.ConfigMapType,
							Ref:  "invalid-ops",
						},
					},
				},
			}
			_, err := resolver.WithOpsManifest(deployment, "default")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to build ops"))
		})

		It("throws an error if interpolate a missing key into a manifest", func() {
			interpolator.InterpolateReturns(nil, errors.New("fake-error"))
			deployment := &bdc.BOSHDeployment{
				Spec: bdc.BOSHDeploymentSpec{
					Manifest: bdc.Manifest{
						Type: bdc.ConfigMapType,
						Ref:  "base-manifest",
					},
					Ops: []bdc.Ops{
						{
							Type: bdc.ConfigMapType,
							Ref:  "missing-key",
						},
					},
				},
			}
			_, err := resolver.WithOpsManifest(deployment, "default")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to interpolate"))
		})

		It("throws an error if containing unsupported ops type", func() {
			interpolator.InterpolateReturns(nil, errors.New("fake-error"))
			deployment := &bdc.BOSHDeployment{
				Spec: bdc.BOSHDeploymentSpec{
					Manifest: bdc.Manifest{
						Type: bdc.ConfigMapType,
						Ref:  "base-manifest",
					},
					Ops: []bdc.Ops{
						{
							Type: "unsupported_type",
							Ref:  "variables",
						},
					},
				},
			}
			_, err := resolver.WithOpsManifest(deployment, "default")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unrecognized ops ref type"))
		})

		It("throws an error if one config map can not be found when contains multi-ops", func() {
			deployment := &bdc.BOSHDeployment{
				Spec: bdc.BOSHDeploymentSpec{
					Manifest: bdc.Manifest{
						Type: bdc.ConfigMapType,
						Ref:  "base-manifest",
					},
					Ops: []bdc.Ops{
						{
							Type: bdc.SecretType,
							Ref:  "opaque-ops",
						},
						{
							Type: bdc.ConfigMapType,
							Ref:  "not-existing",
						},
					},
				},
			}
			_, err := resolver.WithOpsManifest(deployment, "default")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to retrieve ops from configmap"))
		})

		It("throws an error if one secret can not be found when contains multi-ops", func() {
			deployment := &bdc.BOSHDeployment{
				Spec: bdc.BOSHDeploymentSpec{
					Manifest: bdc.Manifest{
						Type: bdc.ConfigMapType,
						Ref:  "base-manifest",
					},
					Ops: []bdc.Ops{
						{
							Type: bdc.SecretType,
							Ref:  "not-existing",
						},
						{
							Type: bdc.ConfigMapType,
							Ref:  "replace-ops",
						},
					},
				},
			}
			_, err := resolver.WithOpsManifest(deployment, "default")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to retrieve ops from secret"))
		})

		It("throws an error if one url ref can not be found when contains multi-ops", func() {
			deployment := &bdc.BOSHDeployment{
				Spec: bdc.BOSHDeploymentSpec{
					Manifest: bdc.Manifest{
						Type: bdc.ConfigMapType,
						Ref:  "base-manifest",
					},
					Ops: []bdc.Ops{
						{
							Type: bdc.ConfigMapType,
							Ref:  "replace-ops",
						},
						{
							Type: bdc.SecretType,
							Ref:  "ops-secret",
						},
						{
							Type: bdc.URLType,
							Ref:  remoteFileServer.URL() + "/not-found-ops.yml",
						},
					},
				},
			}
			_, err := resolver.WithOpsManifest(deployment, "default")

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to retrieve ops from secret"))
		})

		It("replaces implicit variables", func() {
			deployment := &bdc.BOSHDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo-deployment",
				},
				Spec: bdc.BOSHDeploymentSpec{
					Manifest: bdc.Manifest{
						Type: bdc.ConfigMapType,
						Ref:  "manifest-with-vars",
					},
					Ops: []bdc.Ops{},
				},
			}
			m, err := resolver.WithOpsManifest(deployment, "default")

			Expect(err).ToNot(HaveOccurred())
			Expect(m.Variables[1].Options.CommonName).To(Equal("example.com"))
		})
	})
})
