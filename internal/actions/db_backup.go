package actions

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/D8-X/d8x-cli/internal/conn"
	"github.com/D8-X/d8x-cli/internal/styles"
	"github.com/jackc/pgx/v5"
	"github.com/pkg/sftp"
	"github.com/urfave/cli/v2"
)

// Latest available stable postgresql client version to use.
const CurrentMaximumPostgresVersion = 16

// BackupDb performs database backup
func (c *Container) BackupDb(ctx *cli.Context) error {
	styles.PrintCommandTitle("Backing up database...")

	cfg, err := c.ConfigRWriter.Read()
	if err != nil {
		return err
	}

	ip, err := c.HostsCfg.GetMangerPublicIp()
	if err != nil {
		return fmt.Errorf("could not find manager ip: %w", err)
	}

	if len(cfg.DatabaseDSN) == 0 {
		return fmt.Errorf("database dsn is not set in config")
	}

	// Parse the database dsn string
	pgCfg, err := pgx.ParseConfig(cfg.DatabaseDSN)
	if err != nil {
		return fmt.Errorf("parsing database connection string: %w", err)
	}

	// SSH into the manager
	managerConn, err := conn.NewSSHConnection(ip, c.DefaultClusterUserName, c.SshKeyPath)
	if err != nil {
		return fmt.Errorf("creating ssh connection to manager: %w", err)
	}

	pwd, err := c.GetPassword(ctx)
	if err != nil {
		return err
	}

	// Retrieve the postgres version on target database
	fmt.Printf("Determining postgres version\n")
	pgConn, err := pgConnTunnel(managerConn, pgCfg)
	if err != nil {
		return err
	}
	row := pgConn.QueryRow(context.Background(), "select version()")
	versionString := ""
	if err := row.Scan(&versionString); err != nil {
		return fmt.Errorf("retrieving postgres connection version: %w", err)
	}
	re := regexp.MustCompile(`PostgreSQL ([0-9]+)\.?([0-9]+).*`)
	versionMatches := re.FindStringSubmatch(versionString)
	if len(versionMatches) >= 3 {
		majorVersion := versionMatches[1]
		minorVersion := versionMatches[2]
		versionString = majorVersion + "." + minorVersion

	}
	fmt.Printf("Postgres server at %s version: %s\n", pgCfg.Host, versionString)

	// We default to maximum postgresq-client-x version available since it is
	// backwards compatible.
	cmd := "apt-cache search --names-only ^postgresql-client-* | awk '{print $1}'"
	pgClientPackages, err := managerConn.ExecCommand(cmd)
	maxPgVersion := CurrentMaximumPostgresVersion
	if err != nil {
		for _, pkgName := range strings.Split(string(pgClientPackages), "\n") {
			versionStr := strings.TrimPrefix(
				strings.TrimSpace(pkgName),
				"postgresql-client-",
			)
			// Parse only whole int versions
			if version, err := strconv.ParseInt(versionStr, 10, 64); err == nil && maxPgVersion < int(version) {
				maxPgVersion = int(version)
			}
		}
	}
	aptPgClientPackage := "postgresql-client-" + strconv.Itoa(maxPgVersion)

	// Make sure pg dump (postgres 15) is installed on the manager. Let's use
	// the public postgres apt repo and set it up. It contains all latest
	// versions of postgres
	fmt.Printf("Ensuring pg_dump is installed on manager server (%s)\n", aptPgClientPackage)
	installScriptSteps := []string{
		`echo "deb https://apt.postgresql.org/pub/repos/apt $(lsb_release -cs)-pgdg main" > /etc/apt/sources.list.d/pgdg.list`,
		"wget --quiet -O - https://www.postgresql.org/media/keys/ACCC4CF8.asc | apt-key add -",
		"apt-get update -y",
		"apt-get -y install " + aptPgClientPackage,
	}
	for _, s := range installScriptSteps {
		cmd := fmt.Sprintf(`echo "%s" | sudo -S bash -c '%s'`, pwd, s)
		if out, err := managerConn.ExecCommand(cmd); err != nil {
			fmt.Println(string(out))
			return fmt.Errorf("setting up pg_dump on manager server: %w", err)
		}
	}

	// Run pg dump for the database. Create a normal SQL (non-archive) backup
	// file which can be used directly with psql
	backupFileName := fmt.Sprintf("backup-%s-%s.dump.sql", cfg.GetServersLabel(), time.Now().Format("2006-01-02-15-04-05"))
	backupCmd := "PGPASSWORD=%s pg_dump -h %s -p %d -U %s -d %s -f %s"
	backupCmd = fmt.Sprintf(backupCmd, pgCfg.Password, pgCfg.Host, pgCfg.Port, pgCfg.User, pgCfg.Database, backupFileName)

	// TODO handle different versions and incompatible pg_dump and postgres

	fmt.Printf("Creating database %s backup\n", pgCfg.Database)
	if err := managerConn.ExecCommandPiped(backupCmd); err != nil {
		return fmt.Errorf("running pg_dump: %w", err)
	}

	// Use scp or similar library to copy the backup to given location on the
	// local machine
	scp, err := sftp.NewClient(managerConn.GetClient())
	if err != nil {
		return err
	}
	f, err := scp.OpenFile(backupFileName, os.O_RDONLY)
	if err != nil {
		return fmt.Errorf("opening backup file on manager: %w", err)
	}
	fstat, err := f.Stat()
	if err != nil {
		return err
	}
	sizeMb := float64(fstat.Size()) / float64(1024*1024)
	fmt.Printf("Backup file size: %f MB\n", sizeMb)

	// Show a small download animation so user doesn't think that download
	// process is stuck
	stopDownloadSpinner := make(chan struct{})
	go c.TUI.NewSpinner(stopDownloadSpinner, "Downloading backup file to local machine")

	fullBackupPath := backupFileName
	if outDir := ctx.String("output-dir"); outDir != "" {
		fullBackupPath = filepath.Join(outDir, fullBackupPath)
	}
	fullBackupPath, err = filepath.Abs(fullBackupPath)
	if err != nil {
		return err
	}

	// Create backup file in target path
	fout, err := os.OpenFile(fullBackupPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0666)
	if err != nil {
		return err
	}

	if _, err := io.Copy(fout, f); err != nil {
		return err
	}

	stopDownloadSpinner <- struct{}{}

	info := fmt.Sprintf("Database %s backup file was downloaded and copied to %s", pgCfg.Database, fullBackupPath)
	fmt.Println(styles.SuccessText.Render(info))

	// Rm database backup from manager server
	fmt.Println("Removing backup file from server")
	if out, err := managerConn.ExecCommand("rm " + backupFileName); err != nil {
		fmt.Println(string(out))
		return fmt.Errorf("removing backup file from manager: %w", err)
	}

	return nil
}

func pgConnTunnel(manager conn.SSHConnection, pgCfg *pgx.ConnConfig) (*pgx.Conn, error) {
	pgCfg.DialFunc = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return manager.GetClient().DialContext(ctx, network, addr)
	}

	return pgx.ConnectConfig(context.Background(), pgCfg)
}
