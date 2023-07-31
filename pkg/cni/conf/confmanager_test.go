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
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"
)

const (
	testDir      = "./testdata"
	testConf     = "01-test.conf"
	testConfList = "01-test.conflist"
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
			mgr, err := NewCNIConfManager(test.testDir, testConfList, test.exceptionCidrs)
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

func TestStart(t *testing.T) {
	mgr, err := NewCNIConfManager(testDir, testConfList, "")
	if err != nil {
		t.Fatalf("failed to create cni conf manager: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	defer func() {
		cancel()
		wg.Wait()
		_ = os.Remove(filepath.Join(testDir, testConfList))
	}()
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

	// delete existing cni conf file
	_ = os.Remove(filepath.Join(testDir, testConfList))
	time.Sleep(100 * time.Millisecond)
	_, err = os.Stat(filepath.Join(testDir, testConfList))
	if err != nil {
		t.Fatalf("failed to recreate cni conf file after it's deleted: %v", err)
	}

	// rename existing cni conf file
	_ = os.Rename(filepath.Join(testDir, testConfList), filepath.Join(testDir, "test"))
	defer func() {
		_ = os.Remove(filepath.Join(testDir, "test"))
	}()
	time.Sleep(100 * time.Millisecond)
	_, err = os.Stat(filepath.Join(testDir, testConfList))
	if err != nil {
		t.Fatalf("failed to create cni conf file after it's renamed: %v", err)
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
      "socketPath": "/var/run/egressgateway.sock",
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
      "socketPath": "/var/run/egressgateway.sock",
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
			mgr, err := NewCNIConfManager(testDir, confFileName, "10.1.0.0/16,1.2.3.4/32")
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
