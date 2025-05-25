package railcni

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/Mellanox/spectrum-x-operator/internal/controller"
	"github.com/Mellanox/spectrum-x-operator/pkg/exec"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	current "github.com/containernetworking/cni/pkg/types/100"
	"github.com/containernetworking/cni/pkg/version"
)

const (
	defaultPriority = 32768
)

// NetConf represents the CNI configuration
type NetConf struct {
	types.NetConf
	OVSBridge string `json:"ovsBridge"`
}

// parseNetConf parses the CNI configuration
func parseNetConf(data []byte) (*NetConf, error) {
	conf := &NetConf{}
	if err := json.Unmarshal(data, conf); err != nil {
		return nil, fmt.Errorf("failed to parse config: %v", err)
	}
	return conf, nil
}

type RailCNI struct {
	Log   *slog.Logger
	Exec  exec.API
	Flows controller.FlowsAPI
}

func (r *RailCNI) Add(args *skel.CmdArgs) error {
	r.Log.Info("railcni add", "args", args)
	r.Log.Info("railcni add", "args.StdinData", string(args.StdinData))

	conf, err := parseNetConf(args.StdinData)
	if err != nil {
		r.Log.Error("railcni add", "error", err)
		return err
	}

	if err := version.ParsePrevResult(&conf.NetConf); err != nil {
		r.Log.Error("railcni add", "error", err)
		return err
	}

	// Get the previous result from the chain
	var prevResult *current.Result
	if conf.PrevResult != nil {
		prevResult, err = current.NewResultFromResult(conf.PrevResult)
		if err != nil {
			r.Log.Error("railcni add", "error", err)
			return fmt.Errorf("failed to parse prevResult: %v", err)
		}
	} else {
		r.Log.Error("railcni add", "error", "must be called as chained plugin")
		return fmt.Errorf("must be called as chained plugin")
	}

	vf, err := r.getVF(prevResult)
	if err != nil {
		r.Log.Error("railcni add, failed to get vf", "error", err)
		return err
	}

	bridge, err := r.Exec.Execute(fmt.Sprintf("ovs-vsctl port-to-br %s", vf))
	if err != nil {
		r.Log.Error("railcni add", "error", err)
		return fmt.Errorf("failed to get bridge to vf %s: %s", vf, err)
	}

	// TODO: get cookie from the config
	cookie := uint64(0x5)

	pf, err := r.getPF(bridge)
	if err != nil {
		r.Log.Error("railcni add, failed to get pf", "error", err)
		return err
	}

	if len(prevResult.IPs) != 1 {
		r.Log.Error("railcni add, expected single ip, got %d", len(prevResult.IPs))
		return fmt.Errorf("expected single ip, got %d", len(prevResult.IPs))
	}

	// get pod interface mac
	podMac := ""
	for _, iface := range prevResult.Interfaces {
		if iface.Sandbox != "" {
			podMac = iface.Mac
			break
		}
	}

	if podMac == "" {
		r.Log.Error("railcni add, failed to get pod mac", "error", "no pod mac found")
		return fmt.Errorf("no pod mac found")
	}

	if err := r.Flows.AddPodRailFlowsCNI(cookie, vf, bridge, pf, prevResult.IPs[0].Address.IP.String(),
		podMac, prevResult.IPs[0].Gateway.String()); err != nil {
		r.Log.Error("railcni add", "error", err)
		return fmt.Errorf("failed to add pod rail flows: %s", err)
	}

	r.Log.Info("railcni add completed", "vf", vf, "pf", pf)
	// Return the previous result unchanged
	return types.PrintResult(prevResult, prevResult.CNIVersion)
}

func (r *RailCNI) getVF(prevResult *current.Result) (string, error) {
	vf := ""
	for _, iface := range prevResult.Interfaces {
		r.Log.Info("railcni add", "iface", iface.Name)
		if iface.Sandbox == "" {
			vf = iface.Name
			break
		}
	}

	if vf == "" {
		r.Log.Error("railcni add", "error", "no vf found")
		return "", fmt.Errorf("no vf found")
	}

	return vf, nil
}

func (r *RailCNI) getPF(brdige string) (string, error) {
	pf := ""

	// list ports in bridge
	ports, err := r.Exec.Execute(fmt.Sprintf("ovs-vsctl list-ports %s", brdige))
	if err != nil {
		r.Log.Error("railcni add, failed to list ports in bridge", "error", err)
		return pf, fmt.Errorf("failed to list ports in bridge %s: %s", brdige, err)
	}

	// list uplinks
	uplinks, err := r.Exec.Execute("ovs-appctl dpctl/show | grep dpdk | grep -v representor | awk '{print $3}'")
	if err != nil {
		r.Log.Error("railcni add, failed to list uplinks", "error", err)
		return pf, fmt.Errorf("failed to list uplinks: %s", err)
	}

	uplinksList := strings.Split(uplinks, "\n")
	portsList := strings.Split(ports, "\n")

	for _, port := range portsList {
		if slices.Contains(uplinksList, port) {
			pf = port
			break
		}
	}

	if pf == "" {
		r.Log.Error("railcni add, failed to find pf", "error", "no pf found")
		return pf, fmt.Errorf("no pf found")
	}

	return pf, nil
}

func (r *RailCNI) Del(args *skel.CmdArgs) error {
	return nil
}

func (r *RailCNI) Check(args *skel.CmdArgs) error {
	return nil
}
