# Fixes trailing whitespace in files

$files = @(
    ".github/workflows/codeql-analysis.yml",
    ".github/workflows/trivy.yml",
    ".pipeline/scripts/validate-bicep.sh",
    ".pipeline/templates/e2e-steps.yml",
    "Makefile",
    "README.md",
    "SECURITY.md",
    "SUPPORT.md",
    "cmd/kube-egress-gateway-controller/cmd/root.go",
    "config/manager/certmanager/kustomizeconfig.yaml",
    "controllers/daemon/staticgatewayconfiguration_controller.go",
    "controllers/manager/gatewaylbconfiguration_controller.go",
    "controllers/manager/staticgatewayconfiguration_controller.go",
    "docker/base.Dockerfile",
    "docs/cni.md",
    "docs/design.md",
    "docs/install.md",
    "docs/samples/gateway_profile.yaml",
    "docs/troubleshooting.md",
    "hack/generate_release_note.sh",
    "hack/run_e2e.sh",
    "hack/update_helm.sh",
    "helm/kube-egress-gateway/README.md",
    "helm/kube-egress-gateway/templates/gateway-cni-manager.yaml",
    "helm/kube-egress-gateway/templates/gateway-controller-manager.yaml",
    "pkg/azmanager/azmanager.go"
)

foreach ($file in $files) {
    Write-Host "Processing $file"
    # Read file content
    $content = Get-Content -Path $file -Raw
    
    # Replace trailing whitespace
    $newContent = $content -replace '[ \t]+$', '' -replace '\r\n', "`n"
    
    # Write back to the file
    [System.IO.File]::WriteAllText($file, $newContent, [System.Text.UTF8Encoding]::new($false))
}

Write-Host "Whitespace issues fixed in all files."
