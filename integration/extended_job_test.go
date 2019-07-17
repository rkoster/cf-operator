package integration_test

import (
	"fmt"
	"strings"

	"code.cloudfoundry.org/cf-operator/integration/environment"
	ejv1 "code.cloudfoundry.org/cf-operator/pkg/kube/apis/extendedjob/v1alpha1"
	"code.cloudfoundry.org/cf-operator/pkg/kube/util"
	"code.cloudfoundry.org/cf-operator/pkg/kube/util/versionedsecretstore"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("ExtendedJob", func() {
	jobOwnerRef := func(eJob ejv1.ExtendedJob) metav1.OwnerReference {
		return metav1.OwnerReference{
			APIVersion:         "fissile.cloudfoundry.org/v1alpha1",
			Kind:               "ExtendedJob",
			Name:               eJob.Name,
			UID:                eJob.UID,
			Controller:         util.Bool(true),
			BlockOwnerDeletion: util.Bool(true),
		}
	}

	AfterEach(func() {
		env.FlushLog()
	})

	Context("when using an AutoErrandJob", func() {

		var (
			ej        ejv1.ExtendedJob
			tearDowns []environment.TearDownFunc
		)

		BeforeEach(func() {
			ej = env.AutoErrandExtendedJob("autoerrand-job")
		})

		AfterEach(func() {
			Expect(env.TearDownAll(tearDowns)).To(Succeed())
		})

		It("immediately starts the job", func() {
			_, tearDown, err := env.CreateExtendedJob(env.Namespace, ej)
			Expect(err).NotTo(HaveOccurred())
			defer func(tdf environment.TearDownFunc) { Expect(tdf()).To(Succeed()) }(tearDown)

			jobs, err := env.CollectJobs(env.Namespace, fmt.Sprintf("%s=true", ejv1.LabelExtendedJob), 1)
			Expect(err).NotTo(HaveOccurred(), "error waiting for jobs from extendedjob")
			Expect(jobs).To(HaveLen(1))
		})

		Context("when the job succeeded", func() {
			It("cleans up job immediately", func() {
				_, tearDown, err := env.CreateExtendedJob(env.Namespace, ej)
				Expect(err).NotTo(HaveOccurred())
				defer func(tdf environment.TearDownFunc) { Expect(tdf()).To(Succeed()) }(tearDown)

				jobs, err := env.CollectJobs(env.Namespace, fmt.Sprintf("%s=true", ejv1.LabelExtendedJob), 1)
				Expect(err).NotTo(HaveOccurred(), "error waiting for jobs from extendedjob")
				Expect(jobs).To(HaveLen(1))

				err = env.WaitForJobDeletion(env.Namespace, jobs[0].Name)
				Expect(err).ToNot(HaveOccurred())

				By("Checking pod is still there, because delete label is missing")
				Expect(env.PodsDeleted(env.Namespace)).To(BeFalse())
			})

			Context("when pod template has delete label", func() {
				Context("when delete is set to pod", func() {
					BeforeEach(func() {
						ej.Spec.Template.Labels = map[string]string{"delete": "pod"}
					})

					It("removes job's pod", func() {
						_, tearDown, err := env.CreateExtendedJob(env.Namespace, ej)
						Expect(err).NotTo(HaveOccurred())
						defer func(tdf environment.TearDownFunc) { Expect(tdf()).To(Succeed()) }(tearDown)

						jobs, err := env.CollectJobs(env.Namespace, fmt.Sprintf("%s=true", ejv1.LabelExtendedJob), 1)
						Expect(err).NotTo(HaveOccurred(), "error waiting for jobs from extendedjob")
						Expect(jobs).To(HaveLen(1))

						err = env.WaitForJobDeletion(env.Namespace, jobs[0].Name)
						Expect(err).ToNot(HaveOccurred())

						Expect(env.PodsDeleted(env.Namespace)).To(BeTrue())
					})
				})

				Context("when delete is set to something else", func() {
					BeforeEach(func() {
						ej.Spec.Template.Labels = map[string]string{"delete": "something-else"}
					})

					It("keeps the job's pod", func() {
						_, tearDown, err := env.CreateExtendedJob(env.Namespace, ej)
						Expect(err).NotTo(HaveOccurred())
						defer func(tdf environment.TearDownFunc) { Expect(tdf()).To(Succeed()) }(tearDown)

						jobs, err := env.CollectJobs(env.Namespace, fmt.Sprintf("%s=true", ejv1.LabelExtendedJob), 1)
						Expect(err).NotTo(HaveOccurred(), "error waiting for jobs from extendedjob")
						Expect(jobs).To(HaveLen(1))

						err = env.WaitForJobDeletion(env.Namespace, jobs[0].Name)
						Expect(err).ToNot(HaveOccurred())

						Expect(env.PodsDeleted(env.Namespace)).To(BeFalse())
					})
				})
			})
		})

		Context("when the job failed", func() {
			BeforeEach(func() {
				ej.Spec.Template = env.FailingMultiContainerPodTemplate([]string{"echo", "{}"})
			})

			It("cleans it up when the ExtendedJob is deleted", func() {
				_, tearDown, err := env.CreateExtendedJob(env.Namespace, ej)
				Expect(err).NotTo(HaveOccurred())
				defer func(tdf environment.TearDownFunc) { Expect(tdf()).To(Succeed()) }(tearDown)

				jobs, err := env.CollectJobs(env.Namespace, fmt.Sprintf("%s=true", ejv1.LabelExtendedJob), 1)
				Expect(err).NotTo(HaveOccurred(), "error waiting for jobs from extendedjob")
				Expect(jobs).To(HaveLen(1))

				err = env.WaitForJobDeletion(env.Namespace, jobs[0].Name)
				Expect(err).To(HaveOccurred())

				Expect(tearDown()).To(Succeed())
				err = env.WaitForJobDeletion(env.Namespace, jobs[0].Name)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("when configured to update on config change", func() {
			var (
				configMap  corev1.ConfigMap
				secret     corev1.Secret
				tearDownEJ environment.TearDownFunc
			)

			BeforeEach(func() {
				ej.Spec.UpdateOnConfigChange = true
				ej.Spec.Template = env.ConfigPodTemplate()

				configMap = env.DefaultConfigMap("config1")
				secret = env.DefaultSecret("secret1")

				tearDown, err := env.CreateConfigMap(env.Namespace, configMap)
				Expect(err).ToNot(HaveOccurred())
				tearDowns = append(tearDowns, tearDown)

				tearDown, err = env.CreateSecret(env.Namespace, secret)
				Expect(err).ToNot(HaveOccurred())
				tearDowns = append(tearDowns, tearDown)

				_, tearDownEJ, err = env.CreateExtendedJob(env.Namespace, ej)
				Expect(err).NotTo(HaveOccurred())
				tearDowns = append(tearDowns, tearDownEJ)

				_, err = env.WaitForJobExists(env.Namespace, fmt.Sprintf("%s=true", ejv1.LabelExtendedJob))
				Expect(err).NotTo(HaveOccurred())
			})

			Context("when a config content changes", func() {
				It("it creates a new job", func() {
					By("checking if ext job is done")
					eJob, err := env.GetExtendedJob(env.Namespace, ej.Name)
					Expect(err).NotTo(HaveOccurred())
					Expect(eJob.Spec.Trigger.Strategy).To(Equal(ejv1.TriggerDone))
					Expect(env.WaitForLogMsg(env.ObservedLogs, "Deleting succeeded job")).ToNot(HaveOccurred())

					By("modifying config")
					c, _ := env.GetConfigMap(env.Namespace, configMap.Name)
					c.Data["fake-key"] = "fake-value"
					_, _, err = env.UpdateConfigMap(env.Namespace, *c)
					Expect(err).NotTo(HaveOccurred())

					By("checking if job is running")
					jobs, err := env.CollectJobs(env.Namespace, fmt.Sprintf("%s=true", ejv1.LabelExtendedJob), 1)
					Expect(err).NotTo(HaveOccurred())
					Expect(jobs).To(HaveLen(1))
				})
			})
		})

		Context("when enabling update on config change", func() {
			var (
				configMap  corev1.ConfigMap
				secret     corev1.Secret
				tearDownEJ environment.TearDownFunc
			)

			BeforeEach(func() {
				ej.Spec.UpdateOnConfigChange = false
				ej.Spec.Template = env.ConfigPodTemplate()

				configMap = env.DefaultConfigMap("config1")
				secret = env.DefaultSecret("secret1")

				tearDown, err := env.CreateConfigMap(env.Namespace, configMap)
				Expect(err).ToNot(HaveOccurred())
				tearDowns = append(tearDowns, tearDown)

				tearDown, err = env.CreateSecret(env.Namespace, secret)
				Expect(err).ToNot(HaveOccurred())
				tearDowns = append(tearDowns, tearDown)

				_, tearDownEJ, err = env.CreateExtendedJob(env.Namespace, ej)
				Expect(err).NotTo(HaveOccurred())
				tearDowns = append(tearDowns, tearDownEJ)

				_, err = env.WaitForJobExists(env.Namespace, fmt.Sprintf("%s=true", ejv1.LabelExtendedJob))
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when referenced configs are created after the extended job", func() {
			var (
				configMap  corev1.ConfigMap
				secret     corev1.Secret
				tearDownEJ environment.TearDownFunc
				tearDown   environment.TearDownFunc
			)

			BeforeEach(func() {
				ej.Spec.UpdateOnConfigChange = true
				ej.Spec.Template = env.ConfigPodTemplate()

				configMap = env.DefaultConfigMap("config1")
				secret = env.DefaultSecret("secret1")

			})

			Context("when the extended job is created after the config map", func() {
				BeforeEach(func() {
					var err error
					tearDown, err = env.CreateSecret(env.Namespace, secret)
					Expect(err).ToNot(HaveOccurred())

					_, tearDownEJ, err = env.CreateExtendedJob(env.Namespace, ej)
					Expect(err).NotTo(HaveOccurred())
				})

				It("the job starts", func() {
					defer func(tdf environment.TearDownFunc) { Expect(tdf()).To(Succeed()) }(tearDown)
					defer func(tdf environment.TearDownFunc) { Expect(tdf()).To(Succeed()) }(tearDownEJ)

					By("creating the config map")
					tearDown, err := env.CreateConfigMap(env.Namespace, configMap)
					Expect(err).ToNot(HaveOccurred())
					defer func(tdf environment.TearDownFunc) { Expect(tdf()).To(Succeed()) }(tearDown)

					By("waiting for the job to start")
					_, err = env.WaitForJobExists(env.Namespace, fmt.Sprintf("%s=true", ejv1.LabelExtendedJob))
					Expect(err).ToNot(HaveOccurred())
				})
			})

			Context("when the extended job is created after the secret", func() {
				BeforeEach(func() {
					var err error
					tearDown, err = env.CreateConfigMap(env.Namespace, configMap)
					Expect(err).ToNot(HaveOccurred())

					_, tearDownEJ, err = env.CreateExtendedJob(env.Namespace, ej)
					Expect(err).NotTo(HaveOccurred())
				})

				It("the job starts", func() {
					defer func(tdf environment.TearDownFunc) { Expect(tdf()).To(Succeed()) }(tearDown)
					defer func(tdf environment.TearDownFunc) { Expect(tdf()).To(Succeed()) }(tearDownEJ)

					By("creating the secret")
					tearDown, err := env.CreateSecret(env.Namespace, secret)
					Expect(err).ToNot(HaveOccurred())
					defer func(tdf environment.TearDownFunc) { Expect(tdf()).To(Succeed()) }(tearDown)

					By("waiting for the job to start")
					_, err = env.WaitForJobExists(env.Namespace, fmt.Sprintf("%s=true", ejv1.LabelExtendedJob))
					Expect(err).ToNot(HaveOccurred())
				})
			})

			Context("when the extended job is created after several configs", func() {
				BeforeEach(func() {
					var err error
					_, tearDownEJ, err = env.CreateExtendedJob(env.Namespace, ej)
					Expect(err).NotTo(HaveOccurred())
				})

				It("the job starts", func() {
					defer func(tdf environment.TearDownFunc) { Expect(tdf()).To(Succeed()) }(tearDownEJ)

					By("creating the configs")
					tearDown, err := env.CreateSecret(env.Namespace, secret)
					Expect(err).ToNot(HaveOccurred())
					defer func(tdf environment.TearDownFunc) { Expect(tdf()).To(Succeed()) }(tearDown)

					tearDown, err = env.CreateConfigMap(env.Namespace, configMap)
					Expect(err).ToNot(HaveOccurred())
					defer func(tdf environment.TearDownFunc) { Expect(tdf()).To(Succeed()) }(tearDown)

					By("waiting for the job to start")
					_, err = env.WaitForJobExists(env.Namespace, fmt.Sprintf("%s=true", ejv1.LabelExtendedJob))
					Expect(err).ToNot(HaveOccurred())
				})
			})

		})

	})

	Context("when using manually triggered ErrandJob", func() {
		It("does not start a job without Run being set to now", func() {
			ej := env.ErrandExtendedJob("extendedjob")
			ej.Spec.Trigger.Strategy = ejv1.TriggerManual
			_, tearDown, err := env.CreateExtendedJob(env.Namespace, ej)
			Expect(err).NotTo(HaveOccurred())
			defer func(tdf environment.TearDownFunc) { Expect(tdf()).To(Succeed()) }(tearDown)

			exists, err := env.WaitForJobExists(env.Namespace, fmt.Sprintf("%s=true", ejv1.LabelExtendedJob))
			Expect(err).NotTo(HaveOccurred())
			Expect(exists).To(BeFalse())

			latest, err := env.GetExtendedJob(env.Namespace, ej.Name)
			Expect(err).NotTo(HaveOccurred())
			Expect(latest.Spec.Trigger.Strategy).To(Equal(ejv1.TriggerManual))

			err = env.UpdateExtendedJob(env.Namespace, *latest)
			Expect(err).NotTo(HaveOccurred())

			exists, err = env.WaitForJobExists(env.Namespace, fmt.Sprintf("%s=true", ejv1.LabelExtendedJob))
			Expect(err).NotTo(HaveOccurred())
			Expect(exists).To(BeFalse())
		})

		It("starts a job when creating extended job with now", func() {
			ej := env.ErrandExtendedJob("extendedjob")
			_, tearDown, err := env.CreateExtendedJob(env.Namespace, ej)
			Expect(err).NotTo(HaveOccurred())
			defer func(tdf environment.TearDownFunc) { Expect(tdf()).To(Succeed()) }(tearDown)

			jobs, err := env.CollectJobs(env.Namespace, fmt.Sprintf("%s=true", ejv1.LabelExtendedJob), 1)
			Expect(err).NotTo(HaveOccurred(), "error waiting for jobs from extendedjob")
			Expect(jobs).To(HaveLen(1))
		})

		It("starts a job when updating extended job to now", func() {
			ej := env.ErrandExtendedJob("extendedjob")
			ej.Spec.Trigger.Strategy = ejv1.TriggerManual
			_, tearDown, err := env.CreateExtendedJob(env.Namespace, ej)
			Expect(err).NotTo(HaveOccurred())
			defer func(tdf environment.TearDownFunc) { Expect(tdf()).To(Succeed()) }(tearDown)

			latest, err := env.GetExtendedJob(env.Namespace, ej.Name)
			Expect(err).NotTo(HaveOccurred())
			Expect(latest.Spec.Trigger.Strategy).To(Equal(ejv1.TriggerManual))

			latest.Spec.Trigger.Strategy = ejv1.TriggerNow
			err = env.UpdateExtendedJob(env.Namespace, *latest)
			Expect(err).NotTo(HaveOccurred())

			jobs, err := env.CollectJobs(env.Namespace, fmt.Sprintf("%s=true", ejv1.LabelExtendedJob), 1)
			Expect(err).NotTo(HaveOccurred(), "error waiting for jobs from extendedjob")
			Expect(jobs).To(HaveLen(1))

			Expect(jobs[0].GetOwnerReferences()).Should(ContainElement(jobOwnerRef(*latest)))

		})
	})

	Context("when using podstate TriggeredJob", func() {
		Context("when using matchExpressions", func() {
			It("triggers the job", func() {
				ej := *env.MatchExpressionExtendedJob("extendedjob")
				_, tearDown, err := env.CreateExtendedJob(env.Namespace, ej)
				Expect(err).NotTo(HaveOccurred())
				defer func(tdf environment.TearDownFunc) { Expect(tdf()).To(Succeed()) }(tearDown)

				pod := env.LabeledPod("matching", map[string]string{"env": "production"})
				tearDown, err = env.CreatePod(env.Namespace, pod)
				Expect(err).NotTo(HaveOccurred())
				defer func(tdf environment.TearDownFunc) { Expect(tdf()).To(Succeed()) }(tearDown)
				err = env.WaitForPods(env.Namespace, "env=production")
				Expect(err).NotTo(HaveOccurred(), "error waiting for pods")

				By("waiting for the job")
				_, err = env.CollectJobs(env.Namespace, fmt.Sprintf("%s=true", ejv1.LabelExtendedJob), 1)
				Expect(err).NotTo(HaveOccurred(), "error waiting for jobs from extendedjob")
			})
		})

		Context("when using label matchers", func() {

			testLabels := func(key, value string) map[string]string {
				labels := map[string]string{key: value, "test": "true"}
				return labels
			}

			It("does not start a job without matches", func() {
				ej := *env.DefaultExtendedJob("extendedjob")
				_, tearDown, err := env.CreateExtendedJob(env.Namespace, ej)
				Expect(err).NotTo(HaveOccurred())
				defer func(tdf environment.TearDownFunc) { Expect(tdf()).To(Succeed()) }(tearDown)

				exists, err := env.WaitForJobExists(env.Namespace, fmt.Sprintf("%s=true", ejv1.LabelExtendedJob))
				Expect(err).NotTo(HaveOccurred())
				Expect(exists).To(BeFalse())
			})

			It("should pick up new extended jobs", func() {
				By("going into reconciliation without extended jobs")
				pod := env.LabeledPod("nomatch", testLabels("key", "nomatch"))
				tearDown, err := env.CreatePod(env.Namespace, pod)
				Expect(err).NotTo(HaveOccurred())
				defer func(tdf environment.TearDownFunc) { Expect(tdf()).To(Succeed()) }(tearDown)
				err = env.WaitForPods(env.Namespace, "test=true")
				Expect(err).NotTo(HaveOccurred(), "error waiting for pods")

				By("creating a first extended job")
				ej := *env.DefaultExtendedJob("extendedjob")
				_, tearDown, err = env.CreateExtendedJob(env.Namespace, ej)
				Expect(err).NotTo(HaveOccurred())
				defer func(tdf environment.TearDownFunc) { Expect(tdf()).To(Succeed()) }(tearDown)

				By("triggering another reconciliation")
				pod = env.LabeledPod("foo", testLabels("key", "value"))
				tearDown, err = env.CreatePod(env.Namespace, pod)
				Expect(err).NotTo(HaveOccurred())
				defer func(tdf environment.TearDownFunc) { Expect(tdf()).To(Succeed()) }(tearDown)

				By("waiting for the job")
				_, err = env.CollectJobs(env.Namespace, fmt.Sprintf("%s=true", ejv1.LabelExtendedJob), 1)
				Expect(err).NotTo(HaveOccurred(), "error waiting for jobs from extendedjob")

				_, err = env.DeleteJobs(env.Namespace, fmt.Sprintf("%s=true", ejv1.LabelExtendedJob))
				Expect(err).NotTo(HaveOccurred())
				Expect(env.WaitForJobsDeleted(env.Namespace, fmt.Sprintf("%s=true", ejv1.LabelExtendedJob))).To(Succeed())
			})

			It("should start a job for a matched pod", func() {
				// we have to create jobs first, reconciler no-ops if no job matches
				By("creating extended jobs")
				for _, ej := range []ejv1.ExtendedJob{
					*env.DefaultExtendedJob("extendedjob"),
					*env.LongRunningExtendedJob("slowjob"),
					*env.LabelTriggeredExtendedJob(
						"unmatched",
						"ready",
						map[string]string{"unmatched": "unmatched"},
						[]*ejv1.Requirement{},
						[]string{"sleep", "1"},
					),
				} {
					_, tearDown, err := env.CreateExtendedJob(env.Namespace, ej)
					Expect(err).NotTo(HaveOccurred())
					defer func(tdf environment.TearDownFunc) { Expect(tdf()).To(Succeed()) }(tearDown)
				}

				By("creating three pods, two match extended jobs and trigger jobs")
				for _, pod := range []corev1.Pod{
					env.LabeledPod("nomatch", testLabels("key", "nomatch")),
					env.LabeledPod("foo", testLabels("key", "value")),
					env.LabeledPod("bar", testLabels("key", "value")),
				} {
					tearDown, err := env.CreatePod(env.Namespace, pod)
					Expect(err).NotTo(HaveOccurred())
					defer func(tdf environment.TearDownFunc) { Expect(tdf()).To(Succeed()) }(tearDown)
				}

				By("waiting for the jobs")
				jobs, err := env.CollectJobs(env.Namespace, fmt.Sprintf("%s=true", ejv1.LabelExtendedJob), 4)
				Expect(err).NotTo(HaveOccurred(), "error waiting for jobs from extendedjob")
				Expect(jobs).To(HaveLen(4))
				Expect(env.ContainJob(jobs, "extendedjob-foo")).To(Equal(true))
				Expect(env.ContainJob(jobs, "slowjob-foo")).To(Equal(true))
				Expect(env.ContainJob(jobs, "extendedjob-bar")).To(Equal(true))
				Expect(env.ContainJob(jobs, "slowjob-bar")).To(Equal(true))

				By("checking if owner ref is set")
				eJob, err := env.GetExtendedJob(env.Namespace, "extendedjob")
				Expect(err).NotTo(HaveOccurred())
				slowJob, err := env.GetExtendedJob(env.Namespace, "slowjob")
				Expect(err).NotTo(HaveOccurred())

				for _, job := range jobs {
					if strings.Contains(job.GetName(), "extendedjob-") {
						Expect(job.GetOwnerReferences()).Should(ContainElement(jobOwnerRef(*eJob)))
					}
					if strings.Contains(job.GetName(), "slowjob-") {
						Expect(job.GetOwnerReferences()).Should(ContainElement(jobOwnerRef(*slowJob)))
					}
				}
			})

			Context("when persisting output", func() {
				var (
					oej *ejv1.ExtendedJob
				)

				BeforeEach(func() {
					oej = env.OutputExtendedJob("output-job",
						env.MultiContainerPodTemplate([]string{"echo", `{"foo": "1", "bar": "baz"}`}))
				})

				AfterEach(func() {
					env.DeleteSecrets(env.Namespace)
				})

				It("persists output when output persistence is configured", func() {
					_, tearDown, err := env.CreateExtendedJob(env.Namespace, *oej)
					Expect(err).NotTo(HaveOccurred())
					defer func(tdf environment.TearDownFunc) { Expect(tdf()).To(Succeed()) }(tearDown)

					tearDown, err = env.CreatePod(env.Namespace, env.LabeledPod("foo", testLabels("key", "value")))
					Expect(err).NotTo(HaveOccurred())
					defer func(tdf environment.TearDownFunc) { Expect(tdf()).To(Succeed()) }(tearDown)

					By("persisting output for the first container")
					secret, err := env.CollectSecret(env.Namespace, "output-job-output-busybox")
					Expect(err).NotTo(HaveOccurred())
					Expect(string(secret.Data["foo"])).To(Equal("1"))
					Expect(string(secret.Data["bar"])).To(Equal("baz"))

					By("adding the configured labels to the first generated secret")
					Expect(secret.Labels["label-key"]).To(Equal("label-value"))
					Expect(secret.Labels["label-key2"]).To(Equal("label-value2"))

					By("persisting output for the second container")
					secret, err = env.CollectSecret(env.Namespace, "output-job-output-busybox2")
					Expect(err).NotTo(HaveOccurred())
					Expect(string(secret.Data["foo"])).To(Equal("1"))
					Expect(string(secret.Data["bar"])).To(Equal("baz"))

					By("adding the configured labels to the second generated secret")
					Expect(secret.Labels["label-key"]).To(Equal("label-value"))
					Expect(secret.Labels["label-key2"]).To(Equal("label-value2"))
				})

				It("persists output to versioned secret when versioned is configured", func() {
					oej.Spec.Output.Versioned = true
					_, tearDown, err := env.CreateExtendedJob(env.Namespace, *oej)
					Expect(err).NotTo(HaveOccurred())
					defer func(tdf environment.TearDownFunc) { Expect(tdf()).To(Succeed()) }(tearDown)

					tearDown, err = env.CreatePod(env.Namespace, env.LabeledPod("foo", testLabels("key", "value")))
					Expect(err).NotTo(HaveOccurred())
					defer func(tdf environment.TearDownFunc) { Expect(tdf()).To(Succeed()) }(tearDown)

					By("persisting output for the first container")
					secret, err := env.CollectSecret(env.Namespace, "output-job-output-busybox-v1")
					Expect(err).NotTo(HaveOccurred())
					Expect(string(secret.Data["foo"])).To(Equal("1"))
					Expect(string(secret.Data["bar"])).To(Equal("baz"))

					By("adding the configured labels to the first generated secret")
					Expect(secret.Labels["label-key"]).To(Equal("label-value"))
					Expect(secret.Labels["label-key2"]).To(Equal("label-value2"))

					By("persisting output for the second container")
					secret, err = env.CollectSecret(env.Namespace, "output-job-output-busybox2-v1")
					Expect(err).NotTo(HaveOccurred())
					Expect(string(secret.Data["foo"])).To(Equal("1"))
					Expect(string(secret.Data["bar"])).To(Equal("baz"))

					By("adding the configured labels to the second generated secret")
					Expect(secret.Labels["label-key"]).To(Equal("label-value"))
					Expect(secret.Labels["label-key2"]).To(Equal("label-value2"))
				})

				Context("when a secret with the same name already exists", func() {
					BeforeEach(func() {
						oej.Spec.Output.NamePrefix = "overwrite-job-output-"
					})

					It("overwrites the secret", func() {
						existingSecret := env.DefaultSecret("overwrite-job-output-busybox")
						existingSecret.StringData["foo"] = "old"
						existingSecret.StringData["bar"] = "old"
						tearDown, err := env.CreateSecret(env.Namespace, existingSecret)
						defer func(tdf environment.TearDownFunc) { Expect(tdf()).To(Succeed()) }(tearDown)
						Expect(err).ToNot(HaveOccurred())

						_, tearDown, err = env.CreateExtendedJob(env.Namespace, *oej)
						Expect(err).NotTo(HaveOccurred())
						defer func(tdf environment.TearDownFunc) { Expect(tdf()).To(Succeed()) }(tearDown)

						tearDown, err = env.CreatePod(env.Namespace, env.LabeledPod("foo", testLabels("key", "value")))
						Expect(err).NotTo(HaveOccurred())
						defer func(tdf environment.TearDownFunc) { Expect(tdf()).To(Succeed()) }(tearDown)

						// Wait until the output of the second container has been persisted. Then check the first one
						_, err = env.CollectSecret(env.Namespace, "overwrite-job-output-busybox2")
						Expect(err).ToNot(HaveOccurred())

						secret, err := env.CollectSecret(env.Namespace, "overwrite-job-output-busybox")
						Expect(err).ToNot(HaveOccurred())

						Expect(string(secret.Data["foo"])).To(Equal("1"))
						Expect(string(secret.Data["bar"])).To(Equal("baz"))
					})

					It("create new versions of versioned secret", func() {
						oej.Spec.Output.Versioned = true
						existingSecret := env.DefaultSecret("overwrite-job-output-busybox-v1")
						existingSecret.StringData["foo"] = "old"
						existingSecret.StringData["bar"] = "old"
						existingSecret.SetLabels(map[string]string{
							versionedsecretstore.LabelSecretKind: versionedsecretstore.VersionSecretKind,
							versionedsecretstore.LabelVersion:    "1",
						})
						tearDown, err := env.CreateSecret(env.Namespace, existingSecret)
						defer func(tdf environment.TearDownFunc) { Expect(tdf()).To(Succeed()) }(tearDown)
						Expect(err).ToNot(HaveOccurred())

						existingSecret2 := env.DefaultSecret("overwrite-job-output-busybox2-v1")
						existingSecret2.StringData["foo"] = "old"
						existingSecret2.StringData["bar"] = "old"
						existingSecret2.SetLabels(map[string]string{
							versionedsecretstore.LabelSecretKind: versionedsecretstore.VersionSecretKind,
							versionedsecretstore.LabelVersion:    "1",
						})
						tearDown, err = env.CreateSecret(env.Namespace, existingSecret2)
						defer func(tdf environment.TearDownFunc) { Expect(tdf()).To(Succeed()) }(tearDown)
						Expect(err).ToNot(HaveOccurred())

						_, tearDown, err = env.CreateExtendedJob(env.Namespace, *oej)
						Expect(err).NotTo(HaveOccurred())
						defer func(tdf environment.TearDownFunc) { Expect(tdf()).To(Succeed()) }(tearDown)

						tearDown, err = env.CreatePod(env.Namespace, env.LabeledPod("foo", testLabels("key", "value")))
						Expect(err).NotTo(HaveOccurred())
						defer func(tdf environment.TearDownFunc) { Expect(tdf()).To(Succeed()) }(tearDown)

						// Wait until the output of the second container has been persisted. Then check the first one
						_, err = env.CollectSecret(env.Namespace, "overwrite-job-output-busybox2-v2")
						Expect(err).ToNot(HaveOccurred())

						secret, err := env.CollectSecret(env.Namespace, "overwrite-job-output-busybox-v2")
						Expect(err).ToNot(HaveOccurred())

						Expect(string(secret.Data["foo"])).To(Equal("1"))
						Expect(string(secret.Data["bar"])).To(Equal("baz"))
					})
				})

				Context("when the job failed", func() {
					BeforeEach(func() {
						oej.Spec.Output.NamePrefix = "output-job2-output-"
						oej.Spec.Template = env.FailingMultiContainerPodTemplate([]string{"echo", `{"foo": "1", "bar": "baz"}`})
					})

					Context("and WriteOnFailure is false", func() {
						It("does not persist output", func() {
							_, tearDown, err := env.CreateExtendedJob(env.Namespace, *oej)
							Expect(err).NotTo(HaveOccurred())
							defer func(tdf environment.TearDownFunc) { Expect(tdf()).To(Succeed()) }(tearDown)

							tearDown, err = env.CreatePod(env.Namespace, env.LabeledPod("foo", testLabels("key", "value")))
							Expect(err).NotTo(HaveOccurred())
							defer func(tdf environment.TearDownFunc) { Expect(tdf()).To(Succeed()) }(tearDown)

							By("waiting for reconcile")
							err = env.WaitForLogMsg(env.ObservedLogs, "Reconciling job output ")
							Expect(err).NotTo(HaveOccurred())

							By("not persisting output for the first container")
							_, err = env.GetSecret(env.Namespace, "output-job2-output-busybox")
							Expect(err).To(HaveOccurred())
							Expect(err.Error()).To(ContainSubstring("waiting for secret output-job2-output-busybox: secrets \"output-job2-output-busybox\" not found"))
						})
					})

					Context("and WriteOnFailure is true", func() {
						BeforeEach(func() {
							oej.Spec.Output.NamePrefix = "output-job3-output-"
							oej.Spec.Output.WriteOnFailure = true
						})

						It("persists the output", func() {
							_, tearDown, err := env.CreateExtendedJob(env.Namespace, *oej)
							Expect(err).NotTo(HaveOccurred())
							defer func(tdf environment.TearDownFunc) { Expect(tdf()).To(Succeed()) }(tearDown)

							By("triggering the eJob")
							tearDown, err = env.CreatePod(env.Namespace, env.LabeledPod("foo", testLabels("key", "value")))
							Expect(err).NotTo(HaveOccurred())
							defer func(tdf environment.TearDownFunc) { Expect(tdf()).To(Succeed()) }(tearDown)

							By("persisting the output for the first container")
							_, err = env.CollectSecret(env.Namespace, "output-job3-output-busybox")
							Expect(err).ToNot(HaveOccurred())
						})

						It("persists the output to versioned secret", func() {
							oej.Spec.Output.Versioned = true
							_, tearDown, err := env.CreateExtendedJob(env.Namespace, *oej)
							Expect(err).NotTo(HaveOccurred())
							defer func(tdf environment.TearDownFunc) { Expect(tdf()).To(Succeed()) }(tearDown)

							By("triggering the eJob")
							tearDown, err = env.CreatePod(env.Namespace, env.LabeledPod("foo", testLabels("key", "value")))
							Expect(err).NotTo(HaveOccurred())
							defer func(tdf environment.TearDownFunc) { Expect(tdf()).To(Succeed()) }(tearDown)

							By("persisting the output for the first container")
							_, err = env.CollectSecret(env.Namespace, "output-job3-output-busybox-v1")
							Expect(err).ToNot(HaveOccurred())
						})
					})
				})
			})
		})
	})
})
