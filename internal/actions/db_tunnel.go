package actions

import (
	"fmt"
	"io"
	"net"
	"strconv"

	"github.com/D8-X/d8x-cli/internal/conn"
	"github.com/D8-X/d8x-cli/internal/styles"
	"github.com/jackc/pgx/v5"
	"github.com/urfave/cli/v2"
)

func (c *Container) DbTunnel(ctx *cli.Context) error {
	styles.PrintCommandTitle("Esablishing ssh tunnel to database...")

	targetLocalPort := ctx.Args().Get(0)

	// Default to 5432
	port := "5432"
	if targetLocalPort != "" {
		_, err := strconv.ParseInt(targetLocalPort, 10, 64)
		if err != nil {
			return fmt.Errorf("please provde a valid port number: %w", err)
		}
		port = targetLocalPort
	}

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

	// Bind local socket
	l, err := net.Listen("tcp", "127.0.0.1:"+port)
	if err != nil {
		return fmt.Errorf("binding listener on port %s: %w", port, err)
	}

	// SSH into the manager
	managerConn, err := conn.NewSSHConnection(ip, c.DefaultClusterUserName, c.SshKeyPath)
	if err != nil {
		return fmt.Errorf("creating ssh connection to manager: %w", err)
	}

	cpIo := func(w io.Writer, r io.Reader) error {
		_, err := io.Copy(w, r)
		return err
	}

	fmt.Printf("Connecting to database %s via manager server\n\n", pgCfg.Host)

	fmt.Println(styles.SuccessText.Render("Use the following credentials on this machine to connect to the database:"))
	fmt.Printf("Database user: %s\n", pgCfg.User)
	fmt.Printf("Database password: %s\n", pgCfg.Password)
	fmt.Printf("Database name: %s\n", pgCfg.Database)
	fmt.Printf("Database host: %s\n", "127.0.0.1")
	fmt.Printf("Database port: %s\n", port)

	fmt.Println(styles.GrayText.Render("Press Ctrl+C to exit"))

	for {
		conn, err := l.Accept()
		if err != nil {
			return err
		}
		defer conn.Close()

		dbConn, err := managerConn.GetClient().Dial("tcp", pgCfg.Host+":"+strconv.Itoa(int(pgCfg.Port)))
		if err != nil {
			return fmt.Errorf("dialing database on manager: %w", err)
		}

		go cpIo(dbConn, conn)
		go cpIo(conn, dbConn)
	}
}
