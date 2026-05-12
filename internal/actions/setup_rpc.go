package actions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/D8-X/d8x-cli/internal/components"
	"github.com/D8-X/d8x-cli/internal/configs"
	"github.com/D8-X/d8x-cli/internal/conn"
	"github.com/D8-X/d8x-cli/internal/styles"
	"github.com/pkg/sftp"
	"github.com/urfave/cli/v2"
)

const (
	rpcMainRemotePath    = "./trader-backend/rpc.main.json"
	rpcHistoryRemotePath = "./trader-backend/rpc.history.json"
)

func (c *Container) SetupRpc(ctx *cli.Context) error {
	styles.PrintCommandTitle("Manage backend RPC URLs")

	cfg, err := c.ConfigRWriter.Read()
	if err != nil {
		return err
	}
	if _, err := c.EnsureEnvironment(cfg); err != nil {
		return err
	}
	if cfg.ChainId == 0 {
		return fmt.Errorf("no chain_id known for this environment. Run \"setup swarm-deploy\" first")
	}
	chainIdStr := strconv.Itoa(int(cfg.ChainId))

	fmt.Printf("%s Step 1/3: locating manager node\n", arrow)
	managerIp, err := c.HostsCfg.GetMangerPublicIp()
	if err != nil {
		return fmt.Errorf("finding manager ip: %w", err)
	}
	fmt.Printf("  %s manager: %s\n", ok, managerIp)
	fmt.Println(styles.ItalicText.Render("  opening SSH connection..."))
	sshConn, err := c.CreateSSHConn(managerIp, c.DefaultClusterUserName, c.SshKeyPath)
	if err != nil {
		return err
	}
	fmt.Printf("  %s connected as %s\n", ok, c.DefaultClusterUserName)

	fmt.Printf("\n%s Step 2/3: fetching live RPC config from manager\n", arrow)
	fmt.Println(styles.ItalicText.Render("  reading " + rpcMainRemotePath + "..."))
	mainEntries, err := readRemoteRpcFile(sshConn, rpcMainRemotePath)
	if err != nil {
		return err
	}
	fmt.Printf("  %s %s parsed (%d chain entries)\n", ok, rpcMainRemotePath, len(mainEntries))
	fmt.Println(styles.ItalicText.Render("  reading " + rpcHistoryRemotePath + "..."))
	histEntries, err := readRemoteRpcFile(sshConn, rpcHistoryRemotePath)
	if err != nil {
		return err
	}
	fmt.Printf("  %s %s parsed (%d chain entries)\n", ok, rpcHistoryRemotePath, len(histEntries))

	httpPool, wsPool := unionRpcsForChain(cfg.ChainId, mainEntries, histEntries)
	fmt.Printf("  %s union for chain %s: %d HTTP, %d WS\n", ok, chainIdStr, len(httpPool), len(wsPool))

	origHttpPool := append([]string{}, httpPool...)
	origWsPool := append([]string{}, wsPool...)

	fmt.Printf("\n%s Step 3/3: interactive edit (changes applied only on \"Apply and deploy\")\n", arrow)

	for {
		printRpcPools(chainIdStr, httpPool, wsPool)

		opts := []string{"Add HTTP RPC", "Add WS RPC"}
		if len(httpPool) > 0 {
			opts = append(opts, "Delete HTTP RPCs")
		}
		if len(wsPool) > 0 {
			opts = append(opts, "Delete WS RPCs")
		}
		opts = append(opts, "Apply and deploy", "Cancel without saving")

		sel, err := c.TUI.NewSelection(opts,
			components.SelectionOptAllowOnlySingleItem(),
			components.SelectionOptRequireSelection(),
		)
		if err != nil {
			return err
		}
		switch sel[0] {
		case "Add HTTP RPC":
			httpPool, err = addUrl(c, httpPool, "http")
			if err != nil {
				return err
			}
		case "Add WS RPC":
			wsPool, err = addUrl(c, wsPool, "ws")
			if err != nil {
				return err
			}
		case "Delete HTTP RPCs":
			httpPool, err = deleteUrls(c, httpPool)
			if err != nil {
				return err
			}
		case "Delete WS RPCs":
			wsPool, err = deleteUrls(c, wsPool)
			if err != nil {
				return err
			}
		case "Apply and deploy":
			if len(httpPool) == 0 {
				fmt.Println(styles.ErrorText.Render("Refusing to apply: HTTP pool is empty. api and history services would have no RPCs. Add at least one HTTP RPC before applying."))
				continue
			}
			return c.applyRemoteRpcChanges(sshConn, cfg, mainEntries, histEntries, origHttpPool, origWsPool, httpPool, wsPool)
		case "Cancel without saving":
			fmt.Println(styles.ItalicText.Render("No changes applied."))
			return nil
		}
	}
}

func addUrl(c *Container, pool []string, kind string) ([]string, error) {
	var (
		placeholder string
		validator   func(string) bool
		errMsg      string
	)
	switch kind {
	case "http":
		placeholder = "https://your-rpc-provider.com"
		validator = ValidateHttp
		errMsg = "url must start with http:// or https://"
	case "ws":
		placeholder = "wss://your-rpc-provider.com"
		validator = func(s string) bool {
			return strings.HasPrefix(s, "ws://") || strings.HasPrefix(s, "wss://")
		}
		errMsg = "url must start with ws:// or wss://"
	}
	url, err := c.TUI.NewInput(
		components.TextInputOptPlaceholder(placeholder),
		components.TextInputOptDenyEmpty(),
		components.TextInputOptValidation(validator, errMsg),
	)
	if err != nil {
		return pool, err
	}
	url = strings.TrimSpace(url)
	if url == "" {
		return pool, nil
	}
	if slices.Contains(pool, url) {
		fmt.Println(styles.ItalicText.Render("already present, ignoring"))
		return pool, nil
	}
	fmt.Println(styles.SuccessText.Render("  + " + url))
	return append(pool, url), nil
}

func deleteUrls(c *Container, pool []string) ([]string, error) {
	if len(pool) == 0 {
		return pool, nil
	}
	toRemove, err := c.TUI.NewSelection(pool)
	if err != nil {
		return pool, err
	}
	if len(toRemove) == 0 {
		return pool, nil
	}
	for _, r := range toRemove {
		fmt.Println(styles.ErrorText.Render("  - " + r))
	}
	return slices.DeleteFunc(pool, func(u string) bool {
		return slices.Contains(toRemove, u)
	}), nil
}

func (c *Container) applyRemoteRpcChanges(
	sshConn conn.SSHConnection,
	cfg *configs.D8XConfig,
	mainEntries, histEntries []RPCConfigEntry,
	origHttpPool, origWsPool []string,
	httpPool, wsPool []string,
) error {
	chainIdStr := strconv.Itoa(int(cfg.ChainId))

	httpAdded, httpRemoved := diffPools(origHttpPool, httpPool)
	wsAdded, wsRemoved := diffPools(origWsPool, wsPool)

	if len(httpAdded)+len(httpRemoved)+len(wsAdded)+len(wsRemoved) == 0 {
		fmt.Println(styles.ItalicText.Render("No changes vs current manager state. Nothing to apply."))
		return nil
	}

	fmt.Printf("\n%s Pending changes for chain %s\n", arrow, chainIdStr)
	printDiffSection("HTTP", httpAdded, httpRemoved)
	printDiffSection("WS  ", wsAdded, wsRemoved)

	confirmed, err := c.TUI.NewPrompt("Apply these changes to the live cluster?", false)
	if err != nil {
		return err
	}
	if !confirmed {
		fmt.Println(styles.ItalicText.Render("Aborted. No changes applied."))
		return nil
	}

	tmp := *cfg
	tmp.HttpRpcList = map[string][]string{chainIdStr: httpPool}
	tmp.WsRpcList = map[string][]string{chainIdStr: wsPool}

	httpMain, wsMain := DistributeRpcs(0, chainIdStr, &tmp)
	httpHist, wsHist := DistributeRpcs(1, chainIdStr, &tmp)

	newMain := setRpcEntry(mainEntries, cfg.ChainId, httpMain, wsMain)
	newHist := setRpcEntry(histEntries, cfg.ChainId, httpHist, wsHist)

	mainBytes, err := json.MarshalIndent(newMain, "", "\t")
	if err != nil {
		return err
	}
	histBytes, err := json.MarshalIndent(newHist, "", "\t")
	if err != nil {
		return err
	}

	fmt.Printf("\n%s Applying RPC changes to live cluster\n", arrow)
	fmt.Printf("  pool now: %d HTTP, %d WS for chain %s\n", len(httpPool), len(wsPool), chainIdStr)

	rev := time.Now().Format("20060102150405")

	fmt.Printf("\n%s [1/4] backing up existing RPC files on manager\n", arrow)
	for _, p := range []string{rpcMainRemotePath, rpcHistoryRemotePath} {
		bak := fmt.Sprintf("%s.bak.%s", p, rev)
		fmt.Println(styles.ItalicText.Render(fmt.Sprintf("  cp -p %s %s", p, bak)))
		if out, cpErr := sshConn.ExecCommand(fmt.Sprintf("cp -p %s %s", p, bak)); cpErr != nil {
			fmt.Println(string(out))
			return fmt.Errorf("backing up %s on manager: %w", p, cpErr)
		}
		fmt.Printf("  %s backup at %s\n", ok, bak)
	}

	fmt.Printf("\n%s [2/4] uploading new RPC files to manager\n", arrow)
	fmt.Println(styles.ItalicText.Render("  writing " + rpcMainRemotePath + "..."))
	if err := writeRemoteFile(sshConn, rpcMainRemotePath, mainBytes); err != nil {
		return fmt.Errorf("writing %s: %w", rpcMainRemotePath, err)
	}
	fmt.Printf("  %s wrote %s (%d bytes)\n", ok, rpcMainRemotePath, len(mainBytes))
	fmt.Println(styles.ItalicText.Render("  writing " + rpcHistoryRemotePath + "..."))
	if err := writeRemoteFile(sshConn, rpcHistoryRemotePath, histBytes); err != nil {
		return fmt.Errorf("writing %s: %w", rpcHistoryRemotePath, err)
	}
	fmt.Printf("  %s wrote %s (%d bytes)\n", ok, rpcHistoryRemotePath, len(histBytes))

	rolls := []struct {
		configName string
		remotePath string
		services   []string
		targetPath string
	}{
		{"cfg_rpc", rpcMainRemotePath, []string{"api"}, "/cfg_rpc"},
		{"cfg_rpc_history", rpcHistoryRemotePath, []string{"history"}, "/cfg_rpc_history"},
	}

	fmt.Printf("\n%s [3/4] rolling docker swarm services onto new config (revision %s)\n", arrow, rev)
	for _, r := range rolls {
		newName := fmt.Sprintf("%s_%s", r.configName, rev)
		fmt.Println(styles.ItalicText.Render(fmt.Sprintf("  creating docker config %s from %s...", newName, r.remotePath)))
		if out, err := sshConn.ExecCommand(fmt.Sprintf(`docker config create %s %s`, newName, r.remotePath)); err != nil {
			fmt.Println(string(out))
			return fmt.Errorf("creating docker config %s: %w", newName, err)
		}
		fmt.Printf("  %s created %s\n", ok, newName)
		for _, svc := range r.services {
			stackSvc := dockerStackName + "_" + svc
			fmt.Println(styles.ItalicText.Render(fmt.Sprintf("  updating service %s: detach %s, attach %s -> %s", stackSvc, r.configName, newName, r.targetPath)))
			cmd := fmt.Sprintf(
				`docker service update --config-rm %s --config-add source=%s,target=%s %s`,
				r.configName, newName, r.targetPath, stackSvc,
			)
			if err := sshConn.ExecCommandPiped(cmd); err != nil {
				return fmt.Errorf("rolling update of %s failed: %w", stackSvc, err)
			}
			fmt.Printf("  %s service %s now using %s\n", ok, stackSvc, newName)
		}
		fmt.Println(styles.ItalicText.Render(fmt.Sprintf("  refreshing canonical config %s for future \"swarm-deploy\" runs...", r.configName)))
		_, _ = sshConn.ExecCommand(fmt.Sprintf(`docker config rm %s`, r.configName))
		if out, err := sshConn.ExecCommand(fmt.Sprintf(`docker config create %s %s`, r.configName, r.remotePath)); err != nil {
			fmt.Println(string(out))
			fmt.Printf("  %s could not refresh canonical %s; live services use %s and remain healthy. Next \"swarm-deploy\" may need attention.\n", warning, r.configName, newName)
		} else {
			fmt.Printf("  %s canonical %s refreshed\n", ok, r.configName)
		}
	}

	fmt.Printf("\n%s [4/4] verifying service health\n", arrow)
	c.postRpcRolloutHealthCheck(cfg)

	fmt.Printf("\n%s api uses cfg_rpc_%s (%s)\n", ok, rev, rpcMainRemotePath)
	fmt.Printf("%s history uses cfg_rpc_history_%s (%s)\n", ok, rev, rpcHistoryRemotePath)
	fmt.Println(styles.SuccessText.Render("RPC update applied to live cluster."))
	return nil
}

func readRemoteRpcFile(sshConn conn.SSHConnection, remotePath string) ([]RPCConfigEntry, error) {
	body, err := readRemoteFile(sshConn, remotePath)
	if err != nil {
		return nil, fmt.Errorf("reading %s from manager: %w", remotePath, err)
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("%s is empty on manager. Run \"setup swarm-deploy\" first", remotePath)
	}
	var entries []RPCConfigEntry
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", remotePath, err)
	}
	return entries, nil
}

func readRemoteFile(sshConn conn.SSHConnection, remotePath string) ([]byte, error) {
	s, err := sftp.NewClient(sshConn.GetClient())
	if err != nil {
		return nil, err
	}
	defer s.Close()
	f, err := s.Open(remotePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%s does not exist on manager. Run \"setup swarm-deploy\" first", remotePath)
		}
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

func writeRemoteFile(sshConn conn.SSHConnection, remotePath string, content []byte) error {
	s, err := sftp.NewClient(sshConn.GetClient())
	if err != nil {
		return err
	}
	defer s.Close()
	if dir := path.Dir(remotePath); dir != "" && dir != "." {
		if err := s.MkdirAll(dir); err != nil {
			return err
		}
	}
	f, err := s.Create(remotePath)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(content)
	return err
}

func unionRpcsForChain(chainId uint, files ...[]RPCConfigEntry) (httpUrls, wsUrls []string) {
	seenH := map[string]struct{}{}
	seenW := map[string]struct{}{}
	for _, entries := range files {
		for _, e := range entries {
			if e.ChainId != chainId {
				continue
			}
			for _, u := range e.HttpRpcs {
				if u == "" {
					continue
				}
				if _, ok := seenH[u]; !ok {
					seenH[u] = struct{}{}
					httpUrls = append(httpUrls, u)
				}
			}
			if e.WsRpcs != nil {
				for _, u := range *e.WsRpcs {
					if u == "" {
						continue
					}
					if _, ok := seenW[u]; !ok {
						seenW[u] = struct{}{}
						wsUrls = append(wsUrls, u)
					}
				}
			}
		}
	}
	return
}

func setRpcEntry(entries []RPCConfigEntry, chainId uint, httpRpcs, wsRpcs []string) []RPCConfigEntry {
	found := false
	for i, e := range entries {
		if e.ChainId != chainId {
			continue
		}
		entries[i].HttpRpcs = append([]string{}, httpRpcs...)
		if e.WsRpcs != nil || len(wsRpcs) > 0 {
			ws := append([]string{}, wsRpcs...)
			entries[i].WsRpcs = &ws
		}
		found = true
	}
	if !found {
		entry := RPCConfigEntry{
			ChainId:  chainId,
			HttpRpcs: append([]string{}, httpRpcs...),
		}
		if len(wsRpcs) > 0 {
			ws := append([]string{}, wsRpcs...)
			entry.WsRpcs = &ws
		}
		entries = append(entries, entry)
	}
	return entries
}

func (c *Container) postRpcRolloutHealthCheck(cfg *configs.D8XConfig) {
	targets := []configs.D8XServiceName{configs.D8XServiceMainHTTP, configs.D8XServiceHistory}
	for _, name := range targets {
		svc, exists := cfg.Services[name]
		if !exists || svc.HostName == "" {
			fmt.Printf("  %s %s: hostname not configured, skipping check\n", warning, name)
			continue
		}
		scheme := "http"
		if svc.UsesHTTPS {
			scheme = "https"
		}
		url := fmt.Sprintf("%s://%s", scheme, svc.HostName)
		status, took, err := probeService(c.HttpClient, url)
		switch {
		case err != nil:
			fmt.Printf("  %s %s (%s): %s\n", notok, name, url, err)
		case status >= 200 && status < 500:
			fmt.Printf("  %s %s (%s): HTTP %d (%s)\n", ok, name, url, status, took.Round(time.Millisecond))
		default:
			fmt.Printf("  %s %s (%s): HTTP %d (%s)\n", warning, name, url, status, took.Round(time.Millisecond))
		}
	}
}

func probeService(client *http.Client, url string) (int, time.Duration, error) {
	if client == nil {
		client = http.DefaultClient
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, 0, err
	}
	start := time.Now()
	resp, err := client.Do(req)
	took := time.Since(start)
	if err != nil {
		return 0, took, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, took, nil
}

func diffPools(orig, current []string) (added, removed []string) {
	for _, u := range current {
		if !slices.Contains(orig, u) {
			added = append(added, u)
		}
	}
	for _, u := range orig {
		if !slices.Contains(current, u) {
			removed = append(removed, u)
		}
	}
	return
}

func printDiffSection(label string, added, removed []string) {
	if len(added) == 0 && len(removed) == 0 {
		fmt.Printf("  %s: (unchanged)\n", label)
		return
	}
	fmt.Printf("  %s:\n", label)
	for _, u := range added {
		fmt.Println(styles.SuccessText.Render("    + " + u))
	}
	for _, u := range removed {
		fmt.Println(styles.ErrorText.Render("    - " + u))
	}
}

func printRpcPools(chainId string, http, ws []string) {
	fmt.Println()
	fmt.Println(styles.ItalicText.Render(fmt.Sprintf("Chain %s — RPCs configured on manager", chainId)))
	if len(http) == 0 {
		fmt.Println("  HTTP: (none)")
	} else {
		fmt.Println("  HTTP:")
		for i, u := range http {
			fmt.Printf("    %d. %s\n", i+1, u)
		}
	}
	if len(ws) == 0 {
		fmt.Println("  WS:   (none)")
	} else {
		fmt.Println("  WS:")
		for i, u := range ws {
			fmt.Printf("    %d. %s\n", i+1, u)
		}
	}
	fmt.Println()
}
