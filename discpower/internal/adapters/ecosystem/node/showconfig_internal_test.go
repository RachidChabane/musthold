package node

import "testing"

const verifiesShownConfig = " [verifies SP-2657~1]"

func TestReadShownConfig(t *testing.T) {
	const shown = `{
  "configs": [{"rootDir": "/work", "testEnvironment": "/work/node_modules/jest-environment-jsdom/build/index.js"}],
  "globalConfig": {}
}`
	t.Run("the configuration is read past the lines a Next.js configuration logs as it loads"+verifiesShownConfig, func(t *testing.T) {
		stdout := "[Next] Loaded env from /work/.env\n[Next] build mode: standalone\n" + shown + "\n[Next] done\n"
		config, read := readShownConfig([]byte(stdout))
		if !read || len(config.Configs) != 1 || config.Configs[0].RootDir != "/work" {
			t.Fatalf("config %+v, read %v", config, read)
		}
	})
	t.Run("an output with no configuration reads as none"+verifiesShownConfig, func(t *testing.T) {
		if _, read := readShownConfig([]byte("[Next] Loaded env from /work/.env\n{ not json\n")); read {
			t.Fatal("read a configuration")
		}
	})
}
