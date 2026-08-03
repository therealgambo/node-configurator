package sysutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteFileIfChanged(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "file.conf")

	changed, err := WriteFileIfChanged(path, []byte("hello\n"), 0o644)
	if err != nil {
		t.Fatalf("first write: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true on first write")
	}

	changed, err = WriteFileIfChanged(path, []byte("hello\n"), 0o644)
	if err != nil {
		t.Fatalf("second write: %v", err)
	}
	if changed {
		t.Fatal("expected changed=false when content is identical")
	}

	changed, err = WriteFileIfChanged(path, []byte("world\n"), 0o644)
	if err != nil {
		t.Fatalf("third write: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true when content differs")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "world\n" {
		t.Fatalf("unexpected content: %q", data)
	}
}

func TestBackupFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fstab")
	if err := os.WriteFile(path, []byte("original\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := BackupFile(path); err != nil {
		t.Fatalf("backup: %v", err)
	}
	backup := path + ".node-configurator.orig"
	data, err := os.ReadFile(backup)
	if err != nil {
		t.Fatalf("reading backup: %v", err)
	}
	if string(data) != "original\n" {
		t.Fatalf("unexpected backup content: %q", data)
	}

	// Mutate the "live" file and back up again — the original backup must not change.
	if err := os.WriteFile(path, []byte("mutated\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := BackupFile(path); err != nil {
		t.Fatalf("second backup: %v", err)
	}
	data, err = os.ReadFile(backup)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "original\n" {
		t.Fatalf("backup was overwritten: %q", data)
	}
}

func TestSysctlReadWrite(t *testing.T) {
	dir := t.TempDir()
	env := &Env{Root: dir}

	sysctlDir := env.Path("proc", "sys", "net", "core")
	if err := os.MkdirAll(sysctlDir, 0o755); err != nil {
		t.Fatal(err)
	}
	somaxconnPath := filepath.Join(sysctlDir, "somaxconn")
	if err := os.WriteFile(somaxconnPath, []byte("128\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if !env.SysctlExists("net.core.somaxconn") {
		t.Fatal("expected sysctl to exist")
	}
	if env.SysctlExists("net.core.does_not_exist") {
		t.Fatal("expected missing sysctl to report false")
	}

	got, err := env.ReadSysctl("net.core.somaxconn")
	if err != nil {
		t.Fatal(err)
	}
	if got != "128" {
		t.Fatalf("expected 128, got %q", got)
	}

	if err := env.WriteSysctl("net.core.somaxconn", "32768"); err != nil {
		t.Fatal(err)
	}
	got, err = env.ReadSysctl("net.core.somaxconn")
	if err != nil {
		t.Fatal(err)
	}
	if got != "32768" {
		t.Fatalf("expected 32768 after write, got %q", got)
	}

	if err := env.WriteSysctl("net.core.does_not_exist", "1"); err == nil {
		t.Fatal("expected error writing nonexistent sysctl")
	}
}

func TestSysctlPathRejectsTraversal(t *testing.T) {
	env := &Env{Root: "/root"}
	path := env.SysctlPath("net..ipv4.tcp_fin_timeout")
	want := env.Path("proc", "sys", "netipv4", "tcp_fin_timeout")
	if path != want {
		t.Fatalf("expected traversal sequences stripped, got %q want %q", path, want)
	}
	for part := range strings.SplitSeq(path, string(filepath.Separator)) {
		if part == ".." {
			t.Fatalf("path must not contain traversal segments: %q", path)
		}
	}
}

func TestDetectDistro(t *testing.T) {
	dir := t.TempDir()
	env := &Env{Root: dir}
	if err := os.MkdirAll(env.Path("etc"), 0o755); err != nil {
		t.Fatal(err)
	}
	osRelease := "NAME=\"Ubuntu\"\nID=ubuntu\nVERSION_ID=\"22.04\"\n"
	if err := os.WriteFile(env.Path("etc", "os-release"), []byte(osRelease), 0o644); err != nil {
		t.Fatal(err)
	}

	d, err := env.DetectDistro()
	if err != nil {
		t.Fatal(err)
	}
	if d.ID != "ubuntu" || d.VersionID != "22.04" {
		t.Fatalf("unexpected distro: %+v", d)
	}
	if !d.IsUbuntu() {
		t.Fatal("expected IsUbuntu() true")
	}
	if d.IsAmazonLinux2() || d.IsAmazonLinux2023() {
		t.Fatal("expected Amazon Linux checks to be false for Ubuntu")
	}
}

func TestCgroupV2Unified(t *testing.T) {
	dir := t.TempDir()
	env := &Env{Root: dir}
	if env.CgroupV2Unified() {
		t.Fatal("expected false when cgroup.controllers absent")
	}

	if err := os.MkdirAll(env.Path("sys", "fs", "cgroup"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(env.Path("sys", "fs", "cgroup", "cgroup.controllers"), []byte("cpu memory\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !env.CgroupV2Unified() {
		t.Fatal("expected true once cgroup.controllers exists")
	}
}

func TestParseMemInfoTotalKiB(t *testing.T) {
	dir := t.TempDir()
	env := &Env{Root: dir}
	if err := os.MkdirAll(env.Path("proc"), 0o755); err != nil {
		t.Fatal(err)
	}
	meminfo := "MemTotal:       16374912 kB\nMemFree:         1234 kB\n"
	if err := os.WriteFile(env.Path("proc", "meminfo"), []byte(meminfo), 0o644); err != nil {
		t.Fatal(err)
	}

	kib, err := env.ParseMemInfoTotalKiB()
	if err != nil {
		t.Fatal(err)
	}
	if kib != 16374912 {
		t.Fatalf("expected 16374912, got %d", kib)
	}
}
