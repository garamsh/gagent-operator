//go:build e2e
// +build e2e

package e2e

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/garamsh/gagent-operator/test/utils"
)

const (
	// agentTestNamespace holds the Agent under test and everything it produces,
	// so that removing it removes all of them.
	agentTestNamespace = "gagent-e2e"

	agentUnderTest    = "e2e-agent"
	agentPod          = agentUnderTest + "-0"
	credentialsSecret = agentUnderTest + "-credentials"

	// credentialsToken is what the specs look for: in the mounted file, and
	// nowhere in the container's environment.
	credentialsToken = "e2e-token-6f1c9a"

	credentialsMountPath = "/etc/gagent/credentials"
)

// agentManifest is the Agent under test. Its image is loaded onto the node by
// the suite.
var agentManifest = fmt.Sprintf(`
apiVersion: agent.garam.sh/v1alpha1
kind: Agent
metadata:
  name: %s
  namespace: %s
spec:
  image: %s
  credentialsSecretName: %s
  storageSize: 64Mi
`, agentUnderTest, agentTestNamespace, agentImage, credentialsSecret)

// kubectlIn runs kubectl against the namespace the Agent under test lives in.
// The namespace goes in front: after the `--` of an exec it would be an argument
// to the command inside the container instead.
func kubectlIn(args ...string) (string, error) {
	return utils.Run(exec.Command("kubectl", append([]string{"-n", agentTestNamespace}, args...)...))
}

// waitForAgentPod blocks until the Agent's Pod is running, which is what reading
// anything out of that container depends on. It is the subject of the first spec
// and a precondition of the rest, so each one states it rather than inheriting
// it from the spec before.
func waitForAgentPod() {
	GinkgoHelper()

	Eventually(func(g Gomega) {
		phase, err := kubectlIn("get", "pod", agentPod, "-o", "jsonpath={.status.phase}")
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(phase).To(Equal("Running"))
	}, 3*time.Minute, time.Second).Should(Succeed())
}

// execInAgent runs a command inside the agent container of the Agent's Pod.
func execInAgent(args ...string) (string, error) {
	return kubectlIn(append([]string{"exec", agentPod, "-c", "agent", "--"}, args...)...)
}

var _ = Describe("Agent workload", Ordered, func() {
	BeforeAll(func() {
		By("creating the namespace for the Agent under test")
		_, err := utils.Run(exec.Command("kubectl", "create", "ns", agentTestNamespace))
		Expect(err).NotTo(HaveOccurred(), "Failed to create the namespace")

		By("creating the credentials Secret the Agent names")
		_, err = kubectlIn("create", "secret", "generic", credentialsSecret,
			"--from-literal=token="+credentialsToken)
		Expect(err).NotTo(HaveOccurred(), "Failed to create the credentials Secret")

		By("creating the Agent")
		apply := exec.Command("kubectl", "apply", "-f", "-")
		apply.Stdin = strings.NewReader(agentManifest)
		_, err = utils.Run(apply)
		Expect(err).NotTo(HaveOccurred(), "Failed to create the Agent")
	})

	AfterAll(func() {
		By("removing the namespace and everything the Agent produced in it")
		_, _ = utils.Run(exec.Command("kubectl", "delete", "ns", agentTestNamespace,
			"--ignore-not-found", "--timeout=2m"))
	})

	AfterEach(func() {
		if !CurrentSpecReport().Failed() {
			return
		}

		for _, args := range [][]string{
			{"get", "agent", agentUnderTest, "-o", "yaml"},
			{"describe", "statefulset", agentUnderTest},
			{"describe", "pod", agentPod},
			{"get", "events", "--sort-by=.lastTimestamp"},
		} {
			if output, err := kubectlIn(args...); err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "%s:\n%s\n", strings.Join(args, " "), output)
			}
		}
	})

	It("starts the Pod its workload describes and reports Synced on the Agent", func() {
		By("waiting for the Pod to reach Running")
		waitForAgentPod()

		By("reading the Synced condition the operator wrote")
		Eventually(func(g Gomega) {
			status, err := kubectlIn("get", "agent", agentUnderTest,
				"-o", `jsonpath={.status.conditions[?(@.type=="Synced")].status}`)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(status).To(Equal("True"))
		}, time.Minute, time.Second).Should(Succeed())
	})

	It("mounts the credentials as files at the mode asked for, and nowhere in the environment", func() {
		waitForAgentPod()

		By("reading the mode the StatefulSet asks for")
		asked, err := kubectlIn("get", "statefulset", agentUnderTest,
			"-o", "jsonpath={.spec.template.spec.volumes[?(@.name=='credentials')].secret.defaultMode}")
		Expect(err).NotTo(HaveOccurred())
		askedMode, err := strconv.Atoi(asked)
		Expect(err).NotTo(HaveOccurred(), "the StatefulSet asks for no file mode")

		By("reading the mode the file carries inside the container")
		// stat without -L reports the mode of the symlink a Secret volume
		// projects, which is 777 whatever the volume asked for.
		mode, err := execInAgent("stat", "-Lc", "%a", credentialsMountPath+"/token")
		Expect(err).NotTo(HaveOccurred(), "the credentials are not files inside the container")
		Expect(strings.TrimSpace(mode)).To(Equal(strconv.FormatInt(int64(askedMode), 8)))

		By("reading the credential out of the file")
		content, err := execInAgent("cat", credentialsMountPath+"/token")
		Expect(err).NotTo(HaveOccurred())
		Expect(content).To(Equal(credentialsToken))

		By("listing the container's environment")
		environment, err := execInAgent("env")
		Expect(err).NotTo(HaveOccurred())

		// The listing has to be shown to carry what is there before its silence
		// about the credential means anything: HOSTNAME is the Pod's name, so an
		// empty listing, or one from another container, fails here first.
		Expect(environment).To(ContainSubstring("HOSTNAME=" + agentPod))
		Expect(environment).NotTo(ContainSubstring(credentialsToken))
		Expect(environment).NotTo(ContainSubstring("token="))
	})

	It("removes the StatefulSet and its Pod when the Agent is deleted", func() {
		// Without this the workload might not exist yet, and a spec that asserts
		// its absence would pass having never seen it.
		waitForAgentPod()

		By("deleting the Agent")
		_, err := kubectlIn("delete", "agent", agentUnderTest, "--timeout=2m")
		Expect(err).NotTo(HaveOccurred())

		By("waiting for the garbage collector to follow the owner reference")
		for kind, name := range map[string]string{"statefulset": agentUnderTest, "pod": agentPod} {
			Eventually(func(g Gomega) {
				output, err := kubectlIn("get", kind, name)
				g.Expect(err).To(HaveOccurred(), "%s %s still exists", kind, name)
				g.Expect(output).To(ContainSubstring("NotFound"))
			}, 2*time.Minute, time.Second).Should(Succeed())
		}
	})
})
