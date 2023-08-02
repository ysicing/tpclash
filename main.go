package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/irai/packet/fastlog"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	_ "github.com/mritd/logrus"
)

var (
	conf        TPClashConf
	clashConf   *ClashConf
	proxyMode   ProxyMode
	arpHijacker *ARPHijacker
)

var printVer bool
var build string
var commit string
var version string
var clash string

var rootCmd = &cobra.Command{
	Use:   "tpclash",
	Short: "Transparent proxy tool for Clash",
	Run: func(_ *cobra.Command, _ []string) {
		if printVer {
			return
		}

		var err error

		logrus.Info("[main] starting tpclash...")

		cmd := exec.Command(filepath.Join(conf.ClashHome, clashBiName), "-f", conf.ClashConfig, "-d", conf.ClashHome, "-ext-ui", filepath.Join(conf.ClashHome, conf.ClashUI))
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.SysProcAttr = &syscall.SysProcAttr{
			AmbientCaps: []uintptr{CAP_NET_BIND_SERVICE, CAP_NET_ADMIN, CAP_NET_RAW},
		}

		logrus.Debugf("[clash] running cmds: %v", cmd.Args)

		parent, pCancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer pCancel()
		if !conf.AutoExit {
			parent = context.Background()
		}

		ctx, cancel := signal.NotifyContext(parent, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
		defer cancel()
		if err = cmd.Start(); err != nil {
			logrus.Error(err)
			cancel()
		}
		if cmd.Process == nil {
			logrus.Errorf("failed to start clash process: %v", cmd.Args)
			cancel()
		}

		if err = proxyMode.EnableProxy(); err != nil {
			logrus.Fatalf("failed to enable proxy: %v", err)
		}

		if conf.HijackIP != nil {
			if err = arpHijacker.hijack(ctx); err != nil {
				logrus.Fatalf("failed to start arp hijack: %v", err)
			}
		}

		<-time.After(3 * time.Second)
		logrus.Info("[main] 🍄 提莫队长正在待命...")

		<-ctx.Done()
		logrus.Info("[main] 🛑 TPClash 正在停止...")

		if err = proxyMode.DisableProxy(); err != nil {
			logrus.Error(err)
		}

		if cmd.Process != nil {
			if err = cmd.Process.Kill(); err != nil {
				logrus.Error(err)
			}
		}

		logrus.Info("[main] 🛑 TPClash 已关闭!")
	},
}

func init() {
	cobra.EnableCommandSorting = false
	cobra.OnInitialize(tpClashInit)

	rootCmd.PersistentFlags().StringVarP(&conf.ClashHome, "home", "d", "/data/clash", "clash home dir")
	rootCmd.PersistentFlags().StringVarP(&conf.ClashConfig, "config", "c", "/etc/clash.yaml", "clash config local path or remote url")
	rootCmd.PersistentFlags().StringVarP(&conf.ClashUI, "ui", "u", "yacd", "clash dashboard(official|yacd)")
	rootCmd.PersistentFlags().BoolVar(&conf.Debug, "debug", false, "enable debug log")

	rootCmd.PersistentFlags().IPSliceVar(&conf.HijackIP, "hijack-ip", nil, "hijack target IP traffic")
	rootCmd.PersistentFlags().BoolVar(&conf.DisableExtract, "disable-extract", false, "disable extract files")
	rootCmd.PersistentFlags().BoolVar(&conf.AutoExit, "test", false, "run in test mode, exit automatically after 5 minutes")

	rootCmd.PersistentFlags().BoolVarP(&printVer, "version", "v", false, "version for tpclash")

}

func main() {
	cobra.CheckErr(rootCmd.Execute())
}

func tpClashInit() {
	if printVer {
		showVersion()
		return
	}

	// copy static files
	ensureClashFiles()
	ensureSysctl()

	// download remote config
	if strings.HasPrefix(conf.ClashConfig, "http://") ||
		strings.HasPrefix(conf.ClashConfig, "https://") {
		logrus.Info("[main] use remote config...")

		resp, err := http.Get(conf.ClashConfig)
		if err != nil {
			logrus.Fatalf("failed to download remote config: %v", err)
		}

		conf.ClashConfig = filepath.Join(conf.ClashHome, clashRemoteConfig)
		cf, err := os.OpenFile(conf.ClashConfig, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0644)
		if err != nil {
			logrus.Fatalf("failed to create local config file: %v", err)
		}
		defer func() { _ = cf.Close() }()

		_, err = io.Copy(cf, resp.Body)
		if err != nil {
			logrus.Fatalf("failed to save remote config: %v", err)
		}
	}

	// load clash config
	viper.SetConfigFile(conf.ClashConfig)
	viper.SetEnvPrefix("TPCLASH")
	viper.AutomaticEnv()

	logrus.Info("[main] loading config...")
	err := viper.ReadInConfig()
	if err != nil {
		logrus.Fatalf("failed to load clash config: %v", err)
	}

	if clashConf, err = ParseClashConf(); err != nil {
		logrus.Fatal(err)
	}

	if proxyMode, err = NewProxyMode(clashConf, &conf); err != nil {
		logrus.Fatal(err)
	}

	arpHijacker = NewARPHijacker(clashConf, &conf)

	if clashConf.Debug || conf.Debug {
		logrus.SetLevel(logrus.DebugLevel)
	} else {
		fastlog.DefaultIOWriter = io.Discard
	}
}

func showVersion() {
	fmt.Printf("%s\nVersion: %s\nBuild: %s\nClash Core: %s\nCommit: %s\n", logo, version, build, clash, commit)
}
