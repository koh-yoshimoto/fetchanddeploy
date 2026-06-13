package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
)

// Deployer はデプロイコマンドを実行する。
type Deployer struct{}

func NewDeployer() *Deployer { return &Deployer{} }

// Run はrepoのdeployコマンドをpath配下で順次実行する。
// いずれかが失敗した時点で中断しエラーを返す。
func (d *Deployer) Run(repo *Repository, branch, commit string) error {
	if fi, err := os.Stat(repo.Path); err != nil || !fi.IsDir() {
		return fmt.Errorf("作業ディレクトリが存在しません: %s", repo.Path)
	}

	ctx := context.Background()
	if repo.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, repo.Timeout)
		defer cancel()
	}

	// デプロイコマンドから参照できる環境変数。
	env := append(os.Environ(),
		"FAD_REPO="+repo.Name,
		"FAD_BRANCH="+branch,
		"FAD_COMMIT="+commit,
	)

	for i, cmdStr := range repo.Deploy {
		log.Printf("%s: [%d/%d] $ %s", repo.Name, i+1, len(repo.Deploy), cmdStr)

		cmd := exec.CommandContext(ctx, "/bin/sh", "-c", cmdStr)
		cmd.Dir = repo.Path
		cmd.Env = env

		out, err := cmd.CombinedOutput()
		if s := strings.TrimRight(string(out), "\n"); s != "" {
			logIndented(repo.Name, s)
		}
		if err != nil {
			return fmt.Errorf("コマンド失敗 [%d/%d] %q: %w", i+1, len(repo.Deploy), cmdStr, err)
		}
	}
	return nil
}

// logIndented はコマンド出力を行頭インデント付きでログに流す。
func logIndented(name, out string) {
	for _, line := range strings.Split(out, "\n") {
		log.Printf("%s:   | %s", name, line)
	}
}
