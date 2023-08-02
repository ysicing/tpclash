package main

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/spf13/viper"
)

type TPClashConf struct {
	ClashHome   string
	ClashConfig string
	ClashUI     string

	HijackIP       []net.IP
	DisableExtract bool
	AutoExit       bool

	Debug bool
}

type ClashConf struct {
	Debug         bool
	InterfaceName string
}

// ParseClashConf Parses clash configuration and performs necessary checks
// based on proxy mode
func ParseClashConf() (*ClashConf, error) {
	debug := viper.GetString("log-level")
	enhancedMode := viper.GetString("dns.enhanced-mode")
	dnsListen := viper.GetString("dns.listen")
	fakeIPRange := viper.GetString("dns.fake-ip-range")
	interfaceName := viper.GetString("interface-name")
	autoDetectInterface := viper.GetBool("tun.auto-detect-interface")
	tunEnabled := viper.GetBool("tun.enable")
	tunAutoRoute := viper.GetBool("tun.auto-route")
	tunEBPF := viper.GetStringSlice("ebpf.redirect-to-tun")
	routingMark := viper.GetInt("routing-mark")

	// common check
	if strings.ToLower(enhancedMode) != "fake-ip" {
		return nil, fmt.Errorf("only support fake-ip dns mode(dns.enhanced-mode)")
	}

	dnsHost, dnsPort, err := net.SplitHostPort(dnsListen)
	if err != nil {
		return nil, fmt.Errorf("failed to parse clash dns listen config(dns.listen): %v", err)
	}

	dport, err := strconv.Atoi(dnsPort)
	if err != nil {
		return nil, fmt.Errorf("failed to parse clash dns listen config(dns.listen): %v", err)
	}
	if dport < 1 {
		return nil, fmt.Errorf("dns port in clash config is missing(dns.listen)")
	}

	dhost := net.ParseIP(dnsHost)
	if dhost == nil {
		return nil, fmt.Errorf("dns listening address parse failed(dns.listen): is not a valid IP address")
	}

	if interfaceName == "" && !autoDetectInterface {
		return nil, fmt.Errorf("[conf] failed to parse clash interface name(interface-name): interface-name or tun.auto-detect-interface must be set")
	}

	if fakeIPRange == "" {
		return nil, fmt.Errorf("failed to parse clash fake ip range name(dns.fake-ip-range): fake-ip-range must be set")
	}

	if !tunEnabled {
		return nil, fmt.Errorf("tun must be enabled in tun mode(tun.enable)")
	}

if !tunAutoRoute && tunEBPF == nil {
		return nil, fmt.Errorf("[conf] must be enabled auto-route or ebpf in tun mode(tun.auto-route/ebpf.redirect-to-tun)")
	}

	if tunAutoRoute && tunEBPF != nil {
		return nil, fmt.Errorf("[conf] cannot enable auto-route and ebpf at the same time(tun.auto-route/ebpf.redirect-to-tun)")
	}

	if tunEBPF != nil && routingMark == 0 {
		return nil, fmt.Errorf("[conf] ebpf needs to set routing-mark(routing-mark)")
	}

	return &ClashConf{
		Debug:         strings.ToLower(debug) == "debug",
		InterfaceName: interfaceName,
	}, nil
}
