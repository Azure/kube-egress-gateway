/*
MIT License

Copyright (c) Microsoft Corporation.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE
*/
package conf

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fsnotify/fsnotify"

	"github.com/Azure/kube-egress-gateway/pkg/consts"
	"github.com/Azure/kube-egress-gateway/pkg/logger"
)

type Manager struct {
	cniConfDir     string
	cniConfFile    string
	cniConfWatcher *fsnotify.Watcher
	exceptionCidrs []string
}

func NewCNIConfManager(cniConfDir, cniConfFile, exceptionCidrs string) (*Manager, error) {
	cidrs, err := parseCidrs(exceptionCidrs)
	if err != nil {
		return nil, err
	}

	watcher, err := newWatcher(cniConfDir)
	if err != nil {
		return nil, err
	}

	return &Manager{
		cniConfDir:     cniConfDir,
		cniConfFile:    cniConfFile,
		cniConfWatcher: watcher,
		exceptionCidrs: cidrs,
	}, nil
}

func (mgr *Manager) Start(ctx context.Context) error {
	log := logger.GetLogger()
	defer func() {
		log.Info("Stopping cni configuration directory monitoring")
		if err := mgr.cniConfWatcher.Close(); err != nil {
			log.Error(err, "failed to close watcher")
		}
	}()

	log.Info("Installing cni configuration")
	if err := mgr.insertCNIPluginConf(); err != nil {
		return err
	}

	log.Info("Start to watch cni configuration changes", "conf directory", mgr.cniConfDir)
	for {
		select {
		case event := <-mgr.cniConfWatcher.Events:
			if strings.Contains(event.Name, mgr.cniConfFile) && !event.Has(fsnotify.Remove) {
				// ignore our cni conf file change (unless it's deletion) to avoid loop
				continue
			}
			log.Info("Detected changes in cni configuration directory, regenerating...", "change event", event)
			if err := mgr.insertCNIPluginConf(); err != nil {
				log.Error(err, "failed to regenerate cni conf")
			}
		case err := <-mgr.cniConfWatcher.Errors:
			if err != nil {
				log.Error(err, "failed to watch cni configuration directory changes")
			}
		case <-ctx.Done():
			return nil
		}
	}
}

func (mgr *Manager) insertCNIPluginConf() error {
	file, err := findMasterPlugin(mgr.cniConfDir, mgr.cniConfFile)
	if err != nil {
		return err
	}

	ext := filepath.Ext(file)
	var rawList map[string]interface{}
	if ext == ".conflist" {
		rawList, err = mgr.managePluginFromConfList(file)
		if err != nil {
			return err
		}
	} else {
		rawList, err = mgr.managePluginFromConf(file)
		if err != nil {
			return err
		}
	}

	newBytes, err := json.MarshalIndent(rawList, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal bytes into json: %w, bytes: %s", err, string(newBytes))
	}
	tmpFile := filepath.Join(mgr.cniConfDir, mgr.cniConfFile+".tmp")
	err = os.WriteFile(tmpFile, newBytes, 0644)
	if err != nil {
		return fmt.Errorf("failed to write to tmp file: %w", err)
	}
	err = os.Rename(tmpFile, filepath.Join(mgr.cniConfDir, mgr.cniConfFile))
	if err != nil {
		return fmt.Errorf("failed to rename file: %w", err)
	}
	return nil
}

func newWatcher(cniConfDir string) (*fsnotify.Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create new watcher for %q: %v", cniConfDir, err)
	}

	if err = watcher.Add(cniConfDir); err != nil {
		watcher.Close()
		return nil, fmt.Errorf("failed to add watch on %q: %v", cniConfDir, err)
	}

	return watcher, nil
}

func (mgr *Manager) managePluginFromConf(file string) (map[string]interface{}, error) {
	bytes, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read cni config file %s: %w", file, err)
	}
	rawConf, rawList := make(map[string]interface{}), make(map[string]interface{})
	if err = json.Unmarshal(bytes, &rawConf); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cni config from file %s: %w", file, err)
	}

	networkName, ok := rawConf["name"]
	if !ok {
		return nil, fmt.Errorf("failed to find network name in %s", file)
	}
	rawList["name"] = networkName
	delete(rawConf, "name")

	cniVersion, ok := rawConf["cniVersion"]
	if ok {
		cniVersion, ok = cniVersion.(string)
		if !ok {
			return nil, fmt.Errorf("cniVersion (%v) is not in string format", cniVersion)
		}
		rawList["cniVersion"] = cniVersion
		delete(rawConf, "cniVersion")
	}

	var plugins []interface{}
	plugins = append(plugins, rawConf)
	plugins = append(plugins, map[string]interface{}{
		"type":          consts.KubeEgressCNIName,
		"ipam":          map[string]interface{}{"type": consts.KubeEgressIPAMCNIName},
		"excludedCIDRs": mgr.exceptionCidrs,
		"socketPath":    consts.CNISocketPath,
	})
	rawList["plugins"] = plugins
	return rawList, nil
}

func (mgr *Manager) managePluginFromConfList(file string) (map[string]interface{}, error) {
	bytes, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read cni config file %s: %w", file, err)
	}
	rawList := make(map[string]interface{})
	if err = json.Unmarshal(bytes, &rawList); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cni config from file %s: %w", file, err)
	}
	var plugins []interface{}
	plug, ok := rawList["plugins"]
	if !ok {
		return nil, fmt.Errorf("failed to find plugins in cni config file %s", file)
	}
	plugins, ok = plug.([]interface{})
	if !ok {
		return nil, fmt.Errorf("plugins field is not an array in %s", file)
	}
	if len(plugins) == 0 {
		return nil, fmt.Errorf("empty plugin list in cni config file %s", file)
	}

	// remove the plugin if it already exists
	for i, plugin := range plugins {
		p, ok := plugin.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("failed to parse plugin conf in file %s", file)
		}
		cniType, ok := p["type"]
		if !ok {
			return nil, fmt.Errorf("failed to find type in plugin conf in file %s", file)
		}
		cniTypeStr, ok := cniType.(string)
		if ok && strings.EqualFold(cniTypeStr, consts.KubeEgressCNIName) {
			plugins = append(plugins[:i], plugins[i+1:]...)
			break
		}
	}

	// insert kube-egress-gateway-cni at plugins[1]
	plugins = append(plugins[:1], append([]interface{}{map[string]interface{}{
		"type":          consts.KubeEgressCNIName,
		"ipam":          map[string]interface{}{"type": consts.KubeEgressIPAMCNIName},
		"excludedCIDRs": mgr.exceptionCidrs,
		"socketPath":    consts.CNISocketPath,
	}}, plugins[1:]...)...)

	rawList["plugins"] = plugins
	return rawList, nil
}

func findMasterPlugin(cniConfDir, cniConfFile string) (string, error) {
	var confFiles []string
	files, err := os.ReadDir(cniConfDir)
	if err != nil {
		return "", fmt.Errorf("failed to read cni config directory: %w", err)
	}

	for _, file := range files {
		if !file.Type().IsRegular() {
			continue
		}
		if strings.EqualFold(file.Name(), cniConfFile) {
			continue
		}
		fileExtension := filepath.Ext(file.Name())
		if fileExtension == ".conflist" || fileExtension == ".conf" || fileExtension == ".json" {
			confFiles = append(confFiles, file.Name())
		}
	}

	if len(confFiles) == 0 {
		return "", fmt.Errorf("no existing cni plugin configuration file found")
	}
	sort.Strings(confFiles)
	return filepath.Join(cniConfDir, confFiles[0]), nil
}

func parseCidrs(cidrs string) ([]string, error) {
	var res []string
	cidrList := strings.Split(cidrs, ",")
	for _, cidr := range cidrList {
		cidr = strings.TrimSpace(cidr)
		if cidr == "" {
			continue
		}
		_, _, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, fmt.Errorf("cidr %s is not valid: %w", cidr, err)
		}
		res = append(res, cidr)
	}
	return res, nil
}
