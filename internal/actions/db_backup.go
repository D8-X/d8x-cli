package actions

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/D8-X/d8x-cli/internal/conn"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pkg/sftp"
	"github.com/urfave/cli/v2"
)

// BackupDb performs database backup
func (c *Container) BackupDb(ctx *cli.Context) error {
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
	pgCfg, err := pgconn.ParseConfig(cfg.DatabaseDSN)
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
	// Make sure pg dump is installed on the manager
	cmd := fmt.Sprintf(
		`echo "%s" | sudo -S apt-get install -y postgresql-client-common`,
		pwd,
	)
	managerConn.ExecCommand(cmd)

	// Run pg dump for the database
	backupFileName := fmt.Sprintf("backup-%s-%s.dump", cfg.GetServersLabel(), time.Now().Format("2006-01-02-15-04-05"))
	backupCmd := "PGPASSWORD=%s pg_dump -h %s -p %d -U %s -d %s -F c -f %s"
	backupCmd = fmt.Sprintf(backupCmd, pgCfg.Password, pgCfg.Host, pgCfg.Port, pgCfg.User, pgCfg.Database, backupFileName)

	// TODO handle different versions and incompatible pg_dump and postgres

	fmt.Printf("Creating database %s backup\n", pgCfg.Database)
	if err := managerConn.ExecCommandPiped(backupCmd); err != nil {
		return fmt.Errorf("running pg_dump: %w", err)
	}

	fmt.Println("Copying backup file to local machine")
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

	// Create backup file in current dir
	// TOODO use directory flag
	fout, err := os.OpenFile(backupFileName, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0666)
	if err != nil {
		return err
	}
	if _, err := io.Copy(fout, f); err != nil {
		return err
	}

	fmt.Printf("Backup file %s was downloaded", backupFileName)

	return nil
}
