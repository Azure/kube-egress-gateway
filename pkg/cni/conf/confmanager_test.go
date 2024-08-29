// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.
package conf

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/Azure/kube-egress-gateway/pkg/consts"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	testDir                       = "./testdata"
	testConf                      = "01-test.conf"
	testConfList                  = "01-test.conflist"
	testCniUninstallConfigMapName = "cni-uninstall"
	testGrpcPort                  = 5051
)

func TestNewCNIConfManager(t *testing.T) {
	tests := map[string]struct {
		testDir        string
		exceptionCidrs string
		expectErr      bool
		expectedCidrs  []string
	}{
		"NewCNIConfManager should report error when testDir does not exist": {
			testDir:   "testdata1",
			expectErr: true,
		},
		"NewCNIConfManager should report error when exceptionCidrs is not valid": {
			testDir:        "testdata",
			exceptionCidrs: "1000.1.2.3/32",
			expectErr:      true,
		},
		"NewCNIConfManager should return manager as expected": {
			testDir:        "testdata",
			exceptionCidrs: "1.2.3.4/32,10.1.0.0/16",
			expectedCidrs:  []string{"1.2.3.4/32", "10.1.0.0/16"},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			mgr, err := NewCNIConfManager(test.testDir, testConfList, test.exceptionCidrs, testCniUninstallConfigMapName, fake.NewFakeClient(), testGrpcPort)
			defer func() {
				if mgr != nil && mgr.cniConfWatcher != nil {
					_ = mgr.cniConfWatcher.Close()
				}
			}()
			if !test.expectErr {
				if err != nil {
					t.Fatalf("Got unexpected error when creating new cni conf manager: %v", err)
				}
				if mgr.cniConfDir != test.testDir {
					t.Fatalf("mgr's cniConfDir is different: got: %s, expected: %s", mgr.cniConfDir, test.testDir)
				}
				if mgr.cniConfFile != testConfList {
					t.Fatalf("mgr's cniConfFile is different: got: %s, expected: %s", mgr.cniConfFile, testConfList)
				}
				if mgr.cniConfFileTemp != testConfList+".tmp" {
					t.Fatalf("mgr's cniConfFileTemp is different: got: %s, expected: %s", mgr.cniConfFileTemp, testConfList+".tmp")
				}
				if mgr.cniUninstallConfigMapName != testCniUninstallConfigMapName {
					t.Fatalf("mgr's cniUninstallConfigMapName is different: got: %s, expected: %s", mgr.cniUninstallConfigMapName, testCniUninstallConfigMapName)
				}
				if mgr.grpcPort != testGrpcPort {
					t.Fatalf("mgr's grpcPort is different: got: %d, expected: %d", mgr.grpcPort, testGrpcPort)
				}
				if mgr.cniConfWatcher == nil {
					t.Fatalf("mgr's cniConfWatch is nil")
				}
				watches := mgr.cniConfWatcher.WatchList()
				if !reflect.DeepEqual(watches, []string{test.testDir}) {
					t.Fatalf("mgr's cniConfWatcher has unexpected watch list: got: %v, expected: %v", watches, []string{test.testDir})
				}
				if !reflect.DeepEqual(mgr.exceptionCidrs, test.expectedCidrs) {
					t.Fatalf("mgr's exceptionCidrs is different: got: %v, expected: %v", mgr.exceptionCidrs, test.expectedCidrs)
				}
			} else {
				if err == nil {
					t.Fatalf("NewCNIConfManager does not return expected error")
				}
			}
		})
	}
}

func TestIsReady(t *testing.T) {
	tests := map[string]struct {
		fileName    string
		expectReady bool
	}{
		"IsReady() should return true when cni configuration file is found": {
			fileName:    "10-test.conflist",
			expectReady: true,
		},
		"IsReady should return false when cni configuration file is not found": {
			fileName:    "01-test.conflist",
			expectReady: false,
		},
	}
	for name, test := range tests {
		mgr, err := NewCNIConfManager(testDir, test.fileName, "", "", nil, testGrpcPort)
		if err != nil {
			t.Fatalf("failed to create cni conf manager: %v", err)
		}
		ready := mgr.IsReady()
		if ready != test.expectReady {
			t.Fatalf("IsReady returns unexpected result for test %s: got: %v, expected: %v", name, ready, test.expectReady)
		}
	}
}

func TestStart(t *testing.T) {
	os.Setenv(consts.PodNamespaceEnvKey, "default")
	defer os.Unsetenv(consts.PodNamespaceEnvKey)
	client := fake.NewFakeClient(&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: testCniUninstallConfigMapName, Namespace: "default"}, Data: map[string]string{"uninstall": "true"}})
	mgr, err := NewCNIConfManager(testDir, testConfList, "", testCniUninstallConfigMapName, client, testGrpcPort)
	if err != nil {
		t.Fatalf("failed to create cni conf manager: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := mgr.Start(ctx); err != nil {
			t.Errorf("test")
		}
	}()

	// wait for 100 ms and check file
	time.Sleep(100 * time.Millisecond)
	file, err := os.Stat(filepath.Join(testDir, testConfList))
	if err != nil {
		t.Fatalf("failed to find cni conf file after cnimanager is started: %v", err)
	}
	creationTime := file.ModTime()

	// create a random file
	_, err = os.Create(filepath.Join(testDir, "test"))
	if err != nil {
		t.Fatalf("failed to create random file: %v", err)
	}
	defer func() {
		_ = os.Remove(filepath.Join(testDir, "test"))
	}()
	time.Sleep(100 * time.Millisecond)
	file, err = os.Stat(filepath.Join(testDir, testConfList))
	if err != nil {
		t.Fatalf("failed to find target file: %v", err)
	}
	if !file.ModTime().After(creationTime) {
		t.Fatalf("cni conf file is not regenerated")
	}

	cancel()
	wg.Wait()
	_, err = os.Stat(filepath.Join(testDir, testConfList))
	if err == nil || !os.IsNotExist(err) {
		t.Fatalf("expect cni conf file not to be found, actual: %v", err)
	}
}

func TestRemoveCNIPluginConf(t *testing.T) {
	tests := map[string]struct {
		client        client.Client
		expectDeleted bool
	}{
		"confManager should not delete cni conf file upon stop when cniUninstall cm is not found": {
			client:        fake.NewFakeClient(),
			expectDeleted: false,
		},
		"confManager should delete cni conf file upon stop when cniUninstall cm enables cni uninstall": {
			client:        fake.NewFakeClient(&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: testCniUninstallConfigMapName, Namespace: "default"}, Data: map[string]string{"uninstall": "true"}}),
			expectDeleted: true,
		},
		"confManager should not delete cni conf file upon stop when cniUninstall cm disables cni uninstall": {
			client:        fake.NewFakeClient(&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: testCniUninstallConfigMapName, Namespace: "default"}, Data: map[string]string{"uninstall": "false"}}),
			expectDeleted: false,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			os.Setenv(consts.PodNamespaceEnvKey, "default")
			defer os.Unsetenv(consts.PodNamespaceEnvKey)
			mgr, err := NewCNIConfManager(testDir, testConfList, "", testCniUninstallConfigMapName, test.client, testGrpcPort)
			if err != nil {
				t.Fatalf("failed to create cni conf manager: %v", err)
			}

			ctx, cancel := context.WithCancel(context.Background())
			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := mgr.Start(ctx); err != nil {
					t.Errorf("test")
				}
			}()
			cancel()
			wg.Wait()
			_, err = os.Stat(filepath.Join(testDir, testConfList))
			if test.expectDeleted {
				if err == nil || !os.IsNotExist(err) {
					t.Fatalf("expect not found error, got: %v", err)
				}
			} else {
				if err != nil {
					t.Fatalf("expect file to be found, got err: %v", err)
				}
				_ = os.Remove(filepath.Join(testDir, testConfList))
			}
		})
	}
}

func TestInsertCNIPluginConf(t *testing.T) {
	tests := map[string]struct {
		confFile    string
		testFile    string
		expected    string
		expectedErr bool
	}{
		"insertCNIPluginConf should insert new cni plugin configuration in conflist file as expected": {
			confFile: "10-test.conflist",
			testFile: testConfList,
			expected: `{
  "cniVersion": "0.3.1",
  "name": "testnet",
  "plugins": [
    {
      "addIf": "eth0",
      "bridge": "cbr0",
      "hairpinMode": false,
      "ipMasq": false,
      "ipam": {
        "ranges": [
          [
            {
              "subnet": "10.1.0.0/24"
            }
          ]
        ],
        "routes": [
          {
            "dst": "0.0.0.0/0"
          }
        ],
        "type": "host-local"
      },
      "isGateway": true,
      "mtu": 1500,
      "promiscMode": true,
      "type": "bridge"
    },
    {
      "excludedCIDRs": [
        "10.1.0.0/16",
        "1.2.3.4/32"
      ],
      "ipam": {
        "type": "kube-egress-cni-ipam"
      },
      "socketPath": "localhost:5051",
      "type": "kube-egress-cni"
    },
    {
      "capabilities": {
        "portMappings": true
      },
      "externalSetMarkChain": "KUBE-MARK-MASQ",
      "type": "portmap"
    }
  ]
}`,
		},
		"insertCNIPluginConf should insert new cni plugin configuration in conf file as expected": {
			confFile: "10-test.conf",
			testFile: testConf,
			expected: `{
  "cniVersion": "0.3.1",
  "name": "testnet",
  "plugins": [
    {
      "addIf": "eth0",
      "bridge": "cbr0",
      "hairpinMode": false,
      "ipMasq": false,
      "ipam": {
        "ranges": [
          [
            {
              "subnet": "10.1.0.0/24"
            }
          ]
        ],
        "routes": [
          {
            "dst": "0.0.0.0/0"
          }
        ],
        "type": "host-local"
      },
      "isGateway": true,
      "mtu": 1500,
      "promiscMode": true,
      "type": "bridge"
    },
    {
      "excludedCIDRs": [
        "10.1.0.0/16",
        "1.2.3.4/32"
      ],
      "ipam": {
        "type": "kube-egress-cni-ipam"
      },
      "socketPath": "localhost:5051",
      "type": "kube-egress-cni"
    }
  ]
}`,
		},
		"insertCNIPluginConf should return error if existing conflist file has empty plugins": {
			confFile:    "20-fake.conflist",
			testFile:    testConfList,
			expectedErr: true,
		},
		"insertCNIPluginConf should return error if existing conflist file does not have plugins": {
			confFile:    "30-fake.conflist",
			testFile:    testConfList,
			expectedErr: true,
		},
		"insertCNIPluginConf should return error if existing plugin does not have type": {
			confFile:    "40-fake.conflist",
			testFile:    testConfList,
			expectedErr: true,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			confFileName := "50-result.conflist"
			mgr, err := NewCNIConfManager(testDir, confFileName, "10.1.0.0/16,1.2.3.4/32", testCniUninstallConfigMapName, fake.NewFakeClient(), testGrpcPort)
			if err != nil {
				t.Fatalf("failed to create cni conf manager: %v", err)
			}
			defer func() {
				_ = mgr.cniConfWatcher.Close()
			}()

			if err := copyFile(testDir, test.confFile, test.testFile); err != nil {
				t.Fatalf("failed to copy test files: %v", err)
			}
			defer func() {
				_ = os.Remove(filepath.Join(testDir, test.testFile))
				_ = os.Remove(filepath.Join(testDir, confFileName))
			}()

			err = mgr.insertCNIPluginConf()
			if !test.expectedErr {
				if err != nil {
					t.Fatalf("insertCNIPluginConf returns unexpected err: %v", err)
				}
				bytes, err := os.ReadFile(filepath.Join(testDir, confFileName))
				if err != nil {
					t.Fatalf("failed to read result file: %v", err)
				}
				if string(bytes) != test.expected {
					t.Fatalf("insertCNIPluginConf generated unexpected result: expected: %s, got: %s", test.expected, string(bytes))
				}
			} else if test.expectedErr {
				if err == nil {
					t.Fatalf("insertCNIPluginConf does not return expected err")
				}
			}
		})
	}
}

func TestParseCidrs(t *testing.T) {
	tests := map[string]struct {
		input       string
		expected    []string
		expectedErr bool
	}{
		"ParseCidrs should return nil when there's no cidr provided": {
			input: "",
		},
		"ParseCidrs should return expected cidr when there's just one cidr provided": {
			input:    "10.1.0.0/16",
			expected: []string{"10.1.0.0/16"},
		},
		"ParseCidrs should return expected cidrs when there are multiple cidrs provided": {
			input:    "10.1.0.0/16,1.2.3.4/32",
			expected: []string{"10.1.0.0/16", "1.2.3.4/32"},
		},
		"ParseCidrs should trim spaces": {
			input:    "   10.1.0.0/16  ,  1.2.3.4/32, 10.2.2.0/24,",
			expected: []string{"10.1.0.0/16", "1.2.3.4/32", "10.2.2.0/24"},
		},
		"ParseCidrs should report error when an invalid cidr is provided": {
			input:       "1000.2.3.4/32",
			expectedErr: true,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			res, err := parseCidrs(test.input)
			if !test.expectedErr {
				if !reflect.DeepEqual(res, test.expected) {
					t.Fatalf("parseCidrs does not expected result, expected: %#v, got: %#v", test.expected, res)
				}
				if err != nil {
					t.Fatalf("parseCidrs returns unexpected error: %v", err)
				}
			} else if err == nil {
				t.Fatal("parseCidrs does not return expected error")
			}
		})
	}
}

func copyFile(dir, src, dest string) error {
	sf, err := os.Open(filepath.Join(dir, src))
	if err != nil {
		return fmt.Errorf("failed to open src file: %w", err)
	}
	defer sf.Close()
	df, err := os.Create(filepath.Join(dir, dest))
	if err != nil {
		return fmt.Errorf("failed to open dest file: %w", err)
	}
	defer df.Close()
	_, err = io.Copy(df, sf)
	if err != nil {
		return fmt.Errorf("failed to copy file: %w", err)
	}
	return nil
}
