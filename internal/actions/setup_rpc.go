package actions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
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
	if err := c.RequireProvisionedHosts("rpc", "manager"); err != nil {
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

	mainHttp, mainWs := rpcsForChain(mainEntries, cfg.ChainId)
	histHttp, histWs := rpcsForChain(histEntries, cfg.ChainId)
	fmt.Printf("  %s api: %d HTTP, %d WS\n", ok, len(mainHttp), len(mainWs))
	fmt.Printf("  %s history: %d HTTP, %d WS\n", ok, len(histHttp), len(histWs))

	orig := perServicePools{
		mainHttp: append([]string{}, mainHttp...),
		mainWs:   append([]string{}, mainWs...),
		histHttp: append([]string{}, histHttp...),
		histWs:   append([]string{}, histWs...),
	}

	fmt.Printf("\n%s Step 3/3: interactive edit (changes applied only on \"Apply and deploy\")\n", arrow)

	pools := perServicePools{
		mainHttp: mainHttp,
		mainWs:   mainWs,
		histHttp: histHttp,
		histWs:   histWs,
	}

	for {
		printPerServicePools(chainIdStr, &pools)

		opts := []string{"Add HTTP RPC", "Add WS RPC"}
		if len(pools.mainHttp) > 0 || len(pools.histHttp) > 0 {
			opts = append(opts, "Delete HTTP RPCs")
		}
		if len(pools.mainWs) > 0 || len(pools.histWs) > 0 {
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
			if err := addUrlPerService(c, &pools, "http"); err != nil {
				return err
			}
		case "Add WS RPC":
			if err := addUrlPerService(c, &pools, "ws"); err != nil {
				return err
			}
		case "Delete HTTP RPCs":
			if err := deleteUrlsPerService(c, &pools, "http"); err != nil {
				return err
			}
		case "Delete WS RPCs":
			if err := deleteUrlsPerService(c, &pools, "ws"); err != nil {
				return err
			}
		case "Apply and deploy":
			if len(pools.mainHttp) == 0 {
				fmt.Println(styles.ErrorText.Render("Refusing to apply: api HTTP pool is empty. Service would have no RPCs. Add at least one HTTP RPC before applying."))
				continue
			}
			if len(pools.histHttp) == 0 {
				fmt.Println(styles.ErrorText.Render("Refusing to apply: history HTTP pool is empty. Service would have no RPCs. Add at least one HTTP RPC before applying."))
				continue
			}
			return c.applyRemoteRpcChanges(sshConn, cfg, mainEntries, histEntries, &orig, &pools)
		case "Cancel without saving":
			fmt.Println(styles.ItalicText.Render("No changes applied."))
			return nil
		}
	}
}

func addUrlPerService(c *Container, pools *perServicePools, kind string) error {
	target, err := c.TUI.NewSelection(
		[]string{"api only", "history only", "both"},
		components.SelectionOptAllowOnlySingleItem(),
		components.SelectionOptRequireSelection(),
	)
	if err != nil {
		return err
	}
	url, err := promptUrl(c, kind)
	if err != nil {
		return err
	}
	if url == "" {
		return nil
	}
	switch target[0] {
	case "api only":
		if kind == "http" {
			pools.mainHttp = addUrlToPool(pools.mainHttp, url)
		} else {
			pools.mainWs = addUrlToPool(pools.mainWs, url)
		}
	case "history only":
		if kind == "http" {
			pools.histHttp = addUrlToPool(pools.histHttp, url)
		} else {
			pools.histWs = addUrlToPool(pools.histWs, url)
		}
	case "both":
		if kind == "http" {
			pools.mainHttp = addUrlToPool(pools.mainHttp, url)
			pools.histHttp = addUrlToPool(pools.histHttp, url)
		} else {
			pools.mainWs = addUrlToPool(pools.mainWs, url)
			pools.histWs = addUrlToPool(pools.histWs, url)
		}
	}
	return nil
}

func deleteUrlsPerService(c *Container, pools *perServicePools, kind string) error {
	target, err := c.TUI.NewSelection(
		[]string{"api only", "history only", "both"},
		components.SelectionOptAllowOnlySingleItem(),
		components.SelectionOptRequireSelection(),
	)
	if err != nil {
		return err
	}
	switch target[0] {
	case "api only":
		if kind == "http" {
			pools.mainHttp, err = deleteUrls(c, pools.mainHttp)
		} else {
			pools.mainWs, err = deleteUrls(c, pools.mainWs)
		}
	case "history only":
		if kind == "http" {
			pools.histHttp, err = deleteUrls(c, pools.histHttp)
		} else {
			pools.histWs, err = deleteUrls(c, pools.histWs)
		}
	case "both":
		var unionPool []string
		if kind == "http" {
			unionPool = uniqueUnion(pools.mainHttp, pools.histHttp)
		} else {
			unionPool = uniqueUnion(pools.mainWs, pools.histWs)
		}
		if len(unionPool) == 0 {
			return nil
		}
		toRemove, selErr := c.TUI.NewSelection(unionPool)
		if selErr != nil {
			return selErr
		}
		if len(toRemove) == 0 {
			return nil
		}
		for _, r := range toRemove {
			fmt.Println(styles.ErrorText.Render("  - " + r))
		}
		removeFn := func(u string) bool { return slices.Contains(toRemove, u) }
		if kind == "http" {
			pools.mainHttp = slices.DeleteFunc(pools.mainHttp, removeFn)
			pools.histHttp = slices.DeleteFunc(pools.histHttp, removeFn)
		} else {
			pools.mainWs = slices.DeleteFunc(pools.mainWs, removeFn)
			pools.histWs = slices.DeleteFunc(pools.histWs, removeFn)
		}
	}
	return err
}

func addUrlToPool(pool []string, url string) []string {
	if slices.Contains(pool, url) {
		fmt.Println(styles.ItalicText.Render("already present in this service, ignoring"))
		return pool
	}
	fmt.Println(styles.SuccessText.Render("  + " + url))
	return append(pool, url)
}

func uniqueUnion(a, b []string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, u := range a {
		if _, ok := seen[u]; !ok {
			seen[u] = struct{}{}
			out = append(out, u)
		}
	}
	for _, u := range b {
		if _, ok := seen[u]; !ok {
			seen[u] = struct{}{}
			out = append(out, u)
		}
	}
	return out
}

type perServicePools struct {
	mainHttp []string
	mainWs   []string
	histHttp []string
	histWs   []string
}

func promptUrl(c *Container, kind string) (string, error) {
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
		return "", err
	}
	return strings.TrimSpace(url), nil
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
	orig, pools *perServicePools,
) error {
	chainIdStr := strconv.Itoa(int(cfg.ChainId))

	mainHttpA, mainHttpR := diffPools(orig.mainHttp, pools.mainHttp)
	mainWsA, mainWsR := diffPools(orig.mainWs, pools.mainWs)
	histHttpA, histHttpR := diffPools(orig.histHttp, pools.histHttp)
	histWsA, histWsR := diffPools(orig.histWs, pools.histWs)

	totalChanges := len(mainHttpA) + len(mainHttpR) + len(mainWsA) + len(mainWsR) +
		len(histHttpA) + len(histHttpR) + len(histWsA) + len(histWsR)
	if totalChanges == 0 {
		fmt.Println(styles.ItalicText.Render("No changes vs current manager state."))
		forceRoll, err := c.TUI.NewPrompt("Reroll the api and history services anyway (useful after restoring backup files by hand on the manager)?", false)
		if err != nil {
			return err
		}
		if !forceRoll {
			fmt.Println(styles.ItalicText.Render("Nothing to apply."))
			return nil
		}
		fmt.Println(styles.ItalicText.Render("Force rerolling services with the manager's current RPC files..."))
	} else {
		fmt.Printf("\n%s Pending changes for chain %s\n", arrow, chainIdStr)
		fmt.Println("  api:")
		printDiffSection("    HTTP", mainHttpA, mainHttpR)
		printDiffSection("    WS  ", mainWsA, mainWsR)
		fmt.Println("  history:")
		printDiffSection("    HTTP", histHttpA, histHttpR)
		printDiffSection("    WS  ", histWsA, histWsR)

		confirmed, err := c.TUI.NewPrompt("Apply these changes to the live cluster?", false)
		if err != nil {
			return err
		}
		if !confirmed {
			fmt.Println(styles.ItalicText.Render("Aborted. No changes applied."))
			return nil
		}
	}

	newMain := setRpcEntry(mainEntries, cfg.ChainId, pools.mainHttp, pools.mainWs)
	newHist := setRpcEntry(histEntries, cfg.ChainId, pools.histHttp, pools.histWs)

	mainBytes, err := json.MarshalIndent(newMain, "", "\t")
	if err != nil {
		return err
	}
	histBytes, err := json.MarshalIndent(newHist, "", "\t")
	if err != nil {
		return err
	}

	fmt.Printf("\n%s Applying RPC changes to live cluster\n", arrow)
	fmt.Printf("  api now: %d HTTP, %d WS for chain %s\n", len(pools.mainHttp), len(pools.mainWs), chainIdStr)
	fmt.Printf("  history now: %d HTTP, %d WS for chain %s\n", len(pools.histHttp), len(pools.histWs), chainIdStr)

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
			currentName, err := getAttachedConfigName(sshConn, stackSvc, r.targetPath)
			if err != nil {
				return fmt.Errorf("inspecting current config attached to %s at %s: %w", stackSvc, r.targetPath, err)
			}
			if currentName == newName {
				fmt.Printf("  %s service %s already using %s, skipping rollout\n", ok, stackSvc, newName)
				continue
			}
			detachClause := ""
			if currentName != "" {
				detachClause = fmt.Sprintf("--config-rm %s ", currentName)
				fmt.Println(styles.ItalicText.Render(fmt.Sprintf("  updating service %s: detach %s, attach %s -> %s", stackSvc, currentName, newName, r.targetPath)))
			} else {
				fmt.Println(styles.ItalicText.Render(fmt.Sprintf("  updating service %s: attach %s -> %s (no prior config at this target)", stackSvc, newName, r.targetPath)))
			}
			cmd := fmt.Sprintf(
				`docker service update %s--config-add source=%s,target=%s %s`,
				detachClause, newName, r.targetPath, stackSvc,
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

func getAttachedConfigName(sshConn conn.SSHConnection, stackSvc, targetPath string) (string, error) {
	format := fmt.Sprintf(
		`{{range .Spec.TaskTemplate.ContainerSpec.Configs}}{{if eq .File.Name %q}}{{.ConfigName}}{{"\n"}}{{end}}{{end}}`,
		targetPath,
	)
	out, err := sshConn.ExecCommand(fmt.Sprintf(
		`docker service inspect %s --format '%s'`,
		stackSvc, format,
	))
	if err != nil {
		return "", fmt.Errorf("docker service inspect %s: %s (%w)", stackSvc, strings.TrimSpace(string(out)), err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		name := strings.TrimSpace(line)
		if name != "" {
			return name, nil
		}
	}
	return "", nil
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

func rpcsForChain(entries []RPCConfigEntry, chainId uint) (httpUrls, wsUrls []string) {
	seenH := map[string]struct{}{}
	seenW := map[string]struct{}{}
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
	return
}

func setRpcEntry(entries []RPCConfigEntry, chainId uint, httpRpcs, wsRpcs []string) []RPCConfigEntry {
	var indices []int
	hadWsField := false
	for i, e := range entries {
		if e.ChainId == chainId {
			indices = append(indices, i)
			if e.WsRpcs != nil {
				hadWsField = true
			}
		}
	}

	if len(indices) == 0 {
		entry := RPCConfigEntry{
			ChainId:  chainId,
			HttpRpcs: append([]string{}, httpRpcs...),
		}
		if len(wsRpcs) > 0 {
			ws := append([]string{}, wsRpcs...)
			entry.WsRpcs = &ws
		}
		return append(entries, entry)
	}

	keep := indices[0]
	entries[keep].HttpRpcs = append([]string{}, httpRpcs...)
	if hadWsField || len(wsRpcs) > 0 {
		ws := append([]string{}, wsRpcs...)
		entries[keep].WsRpcs = &ws
	} else {
		entries[keep].WsRpcs = nil
	}

	for i := len(indices) - 1; i > 0; i-- {
		idx := indices[i]
		entries = append(entries[:idx], entries[idx+1:]...)
	}
	return entries
}

func (c *Container) postRpcRolloutHealthCheck(cfg *configs.D8XConfig) {
	hosts := c.discoverServiceHosts(cfg)
	if len(hosts) == 0 {
		fmt.Printf("  %s no service hostnames discovered (cfg.Services empty and no %s/sites.conf reachable); skipping health probe\n", warning, c.SelectedEnv)
		return
	}

	for _, h := range hosts {
		scheme := "http"
		if h.https {
			scheme = "https"
		}
		url := fmt.Sprintf("%s://%s", scheme, h.host)
		status, took, err := probeService(c.HttpClient, url)
		switch {
		case err != nil:
			fmt.Printf("  %s %s (%s): %s\n", notok, h.label, url, err)
		case status >= 200 && status < 500:
			fmt.Printf("  %s %s (%s): HTTP %d (%s)\n", ok, h.label, url, status, took.Round(time.Millisecond))
		default:
			fmt.Printf("  %s %s (%s): HTTP %d (%s)\n", warning, h.label, url, status, took.Round(time.Millisecond))
		}
	}
}

type discoveredHost struct {
	label string
	host  string
	https bool
}

func (c *Container) discoverServiceHosts(cfg *configs.D8XConfig) []discoveredHost {
	seen := map[string]struct{}{}
	var out []discoveredHost

	for _, svc := range cfg.Services {
		if svc.HostName == "" {
			continue
		}
		if _, dup := seen[svc.HostName]; dup {
			continue
		}
		seen[svc.HostName] = struct{}{}
		out = append(out, discoveredHost{
			label: string(svc.Name),
			host:  svc.HostName,
			https: svc.UsesHTTPS,
		})
	}

	token := os.Getenv("GITHUB_TOKEN")
	if token == "" || c.SelectedEnv == "" {
		return out
	}
	sites, err := ghReadFile(token, c.SelectedEnv+"/sites.conf")
	if err != nil {
		fmt.Printf("  %s could not fetch %s/sites.conf (%s); probing only entries in cfg.Services\n", warning, c.SelectedEnv, err)
		return out
	}
	for _, host := range extractAllServerNames(sites.Content) {
		if _, dup := seen[host]; dup {
			continue
		}
		seen[host] = struct{}{}
		label := strings.Split(host, ".")[0]
		out = append(out, discoveredHost{
			label: label,
			host:  host,
			https: true,
		})
	}
	return out
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

func printPerServicePools(chainId string, p *perServicePools) {
	fmt.Println()
	fmt.Println(styles.ItalicText.Render(fmt.Sprintf("Chain %s, RPCs configured on manager:", chainId)))
	printOneServicePool("api", p.mainHttp, p.mainWs)
	printOneServicePool("history", p.histHttp, p.histWs)
	fmt.Println()
}

func printOneServicePool(label string, http, ws []string) {
	fmt.Printf("  %s:\n", label)
	if len(http) == 0 {
		fmt.Println("    HTTP: (none)")
	} else {
		fmt.Println("    HTTP:")
		for i, u := range http {
			fmt.Printf("      %d. %s\n", i+1, u)
		}
	}
	if len(ws) == 0 {
		fmt.Println("    WS:   (none)")
	} else {
		fmt.Println("    WS:")
		for i, u := range ws {
			fmt.Printf("      %d. %s\n", i+1, u)
		}
	}
}
