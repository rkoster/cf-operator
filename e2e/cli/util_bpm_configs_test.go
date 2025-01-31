package cli_test

import (
	"encoding/json"
	"os/exec"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	yaml "gopkg.in/yaml.v2"

	"code.cloudfoundry.org/cf-operator/pkg/bosh/bpm"
)

var _ = Describe("bpm-configs", func() {
	var (
		manifestPath string
	)

	act := func(manifestPath string) (session *gexec.Session, err error) {
		args := []string{"util", "bpm-configs", "-m", manifestPath, "-b", assetPath, "-g", "log-api"}
		cmd := exec.Command(cliPath, args...)
		session, err = gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
		return
	}

	Context("when manifest exists", func() {
		BeforeEach(func() {
			manifestPath = assetPath + "/gatherManifest.yml"
		})

		It("prints the bpm configs to stdout", func() {
			session, err := act(manifestPath)
			Expect(err).ToNot(HaveOccurred())

			Eventually(session).Should(gexec.Exit(0))

			output := session.Out.Contents()
			jsonOutput := map[string]string{}
			err = json.Unmarshal(output, &jsonOutput)
			Expect(err).ToNot(HaveOccurred())

			configs := bpm.Configs{}
			err = yaml.Unmarshal([]byte(jsonOutput["bpm.yaml"]), &configs)
			Expect(err).ToNot(HaveOccurred())

			config := configs["loggregator_trafficcontroller"]
			Expect(len(config.Processes)).To(Equal(1))
			Expect(config.Processes[0].Executable).To(Equal("/var/vcap/packages/loggregator_trafficcontroller/trafficcontroller"))
		})
	})
})
