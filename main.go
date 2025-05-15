package main

import (
	"AgentSmith-HUB/rules_engine"
	"fmt"
	"github.com/bytedance/sonic"
	"io/ioutil"
	"os"
)

var testData = "{\n\"uid\":\"0\",\n\"data_type\":\"59\",\n\"run_path\":\"/tmp\",\n\"exe\":\"/opt/ltp/testcases/bin/growfiles\",\n\"argv\":\"growfiles -W gf26 -D 0 -b -i 0 -L 60 -u -B 1000b -e 1 -r 128-32768:128 -R 512-64000 -T 4 -f gfsmallio-35861 -d /tmp/ltp-Ujxl8kKsKY \",\n\"pid\":\"35861\",\n\"ppid\":\"35711\",\n\"pgid\":\"35861\",\n\"tgid\":\"35861\",\n\"comm\":\"growfiles\",\n\"nodename\":\"test\",\n\"stdin\":\"/dev/pts/1\",\n\"stdout\":\"/dev/pts/1\",\n\"sessionid\":\"3\",\n\"sip\":\"192.168.165.1\",\n\"sport\":\"61726\",\n\"dip\":\"192.168.165.128\",\n\"dport\":\"22\",\n\"sa_family\":\"1\",\n\"pid_tree\":\"1(systemd)->1384(sshd)->2175(sshd)->2177(bash)->2193(fish)->35552(runltp)->35711(ltp-pan)->35861(growfiles)\",\n\"tty_name\":\"pts1\",\n\"socket_process_pid\":\"2175\",\n\"socket_process_exe\":\"/usr/sbin/sshd\",\n\"SSH_CONNECTION\":\"192.168.165.1 61726 192.168.165.128 22\",\n\"LD_PRELOAD\":\"/root/ldpreload/test.so\",\n\"user\":\"root\",\n\"time\":\"1579575429143\",\n\"local_ip\":\"192.168.165.128\",\n\"hostname\":\"test\",\n\"exe_md5\":\"01272152d4901fd3c2efacab5c0e38e5\",\n\"socket_process_exe_md5\":\"686cd72b4339da33bfb6fe8fb94a301f\"\n}"

func initRuleset(filePath string) *rules_engine.Ruleset {
	xmlFile, err := os.Open(filePath)
	if err != nil {
		panic(err)
	}
	defer xmlFile.Close()

	rawRuleset, err := ioutil.ReadAll(xmlFile)
	if err != nil {
		panic(err)
	}

	ruleset, err := rules_engine.ParseRulesetFromByte(rawRuleset)
	if err != nil {
		fmt.Println(err)
	}
	return ruleset
}

func main() {
	ruleset := initRuleset("test/ruleset/test.xml")

	data := make(map[string]interface{}, 10)
	_ = sonic.Unmarshal([]byte(testData), &data)

	ruleset.EngineCheck(data)
}
