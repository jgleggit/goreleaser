package exec

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goreleaser/goreleaser/internal/artifact"
	"github.com/goreleaser/goreleaser/internal/pipe"
	"github.com/goreleaser/goreleaser/internal/testctx"
	"github.com/goreleaser/goreleaser/pkg/config"
	"github.com/stretchr/testify/require"
)

func TestExecute(t *testing.T) {
	ctx := testctx.NewWithCfg(config.Project{
		ProjectName: "blah",
		Env: []string{
			"TEST_A_SECRET=x",
			"TEST_A_USERNAME=u2",
		},
	}, testctx.WithVersion("2.1.0"))

	// Preload artifacts
	folder := t.TempDir()
	for _, a := range []struct {
		id  string
		ext string
		typ artifact.Type
	}{
		{"debpkg", "deb", artifact.LinuxPackage},
		{"binary", "bin", artifact.Binary},
		{"archive", "tar", artifact.UploadableArchive},
		{"ubinary", "ubi", artifact.UploadableBinary},
		{"checksum", "sum", artifact.Checksum},
		{"signature", "sig", artifact.Signature},
		{"signature", "pem", artifact.Certificate},
	} {
		file := filepath.Join(folder, "a."+a.ext)
		require.NoError(t, os.WriteFile(file, []byte("lorem ipsum"), 0o644))
		ctx.Artifacts.Add(&artifact.Artifact{
			Name:   "a." + a.ext,
			Goos:   "linux",
			Goarch: "amd64",
			Path:   file,
			Type:   a.typ,
			Extra: map[string]interface{}{
				artifact.ExtraID: a.id,
			},
		})
	}

	ctx.Artifacts.Add(&artifact.Artifact{
		Name:   "foo/bar:amd64",
		Goos:   "linux",
		Goarch: "amd64",
		Path:   "foo/bar:amd64",
		Type:   artifact.DockerImage,
		Extra: map[string]interface{}{
			artifact.ExtraID: "img",
		},
	})
	ctx.Artifacts.Add(&artifact.Artifact{
		Name: "foo/bar",
		Path: "foo/bar",
		Type: artifact.DockerManifest,
		Extra: map[string]interface{}{
			artifact.ExtraID: "mnf",
		},
	})

	osEnv := func(ignores ...string) []string {
		var result []string
	outer:
		for _, key := range passthroughEnvVars {
			for _, ignore := range ignores {
				if key == ignore {
					continue outer
				}
			}
			if value := os.Getenv(key); value != "" {
				result = append(result, key+"="+value)
			}
		}
		return result
	}

	testCases := []struct {
		name       string
		publishers []config.Publisher
		expectErr  error
	}{
		{
			"filter by IDs",
			[]config.Publisher{
				{
					Name: "test",
					IDs:  []string{"archive"},
					Cmd:  MockCmd + " {{ .ArtifactName }}",
					Env: []string{
						MarshalMockEnv(&MockData{
							AnyOf: []MockCall{
								{ExpectedArgs: []string{"a.tar"}, ExitCode: 0, ExpectedEnv: osEnv()},
							},
						}),
					},
				},
			},
			nil,
		},
		{
			"no filter",
			[]config.Publisher{
				{
					Name:    "test",
					Cmd:     MockCmd + " {{ .ArtifactName }}",
					Disable: "false",
					Env: []string{
						MarshalMockEnv(&MockData{
							AnyOf: []MockCall{
								{ExpectedArgs: []string{"a.deb"}, ExitCode: 0, ExpectedEnv: osEnv()},
								{ExpectedArgs: []string{"a.ubi"}, ExitCode: 0, ExpectedEnv: osEnv()},
								{ExpectedArgs: []string{"a.tar"}, ExitCode: 0, ExpectedEnv: osEnv()},
								{ExpectedArgs: []string{"foo/bar"}, ExitCode: 0, ExpectedEnv: osEnv()},
								{ExpectedArgs: []string{"foo/bar:amd64"}, ExitCode: 0, ExpectedEnv: osEnv()},
							},
						}),
					},
				},
			},
			nil,
		},
		{
			"disabled",
			[]config.Publisher{
				{
					Name:    "test",
					Cmd:     MockCmd + " {{ .ArtifactName }}",
					Disable: "true",
					Env:     []string{},
				},
			},
			pipe.ErrSkip{},
		},
		{
			"disabled invalid tmpl",
			[]config.Publisher{
				{
					Name:    "test",
					Cmd:     MockCmd + " {{ .ArtifactName }}",
					Disable: "{{ .NOPE }}",
					Env:     []string{},
				},
			},
			fmt.Errorf(`template: tmpl:1:3: executing "tmpl" at <.NOPE>`),
		},
		{
			"include checksum",
			[]config.Publisher{
				{
					Name:     "test",
					Checksum: true,
					Cmd:      MockCmd + " {{ .ArtifactName }}",
					Env: []string{
						MarshalMockEnv(&MockData{
							AnyOf: []MockCall{
								{ExpectedArgs: []string{"a.deb"}, ExitCode: 0, ExpectedEnv: osEnv()},
								{ExpectedArgs: []string{"a.ubi"}, ExitCode: 0, ExpectedEnv: osEnv()},
								{ExpectedArgs: []string{"a.tar"}, ExitCode: 0, ExpectedEnv: osEnv()},
								{ExpectedArgs: []string{"a.sum"}, ExitCode: 0, ExpectedEnv: osEnv()},
								{ExpectedArgs: []string{"foo/bar"}, ExitCode: 0, ExpectedEnv: osEnv()},
								{ExpectedArgs: []string{"foo/bar:amd64"}, ExitCode: 0, ExpectedEnv: osEnv()},
							},
						}),
					},
				},
			},
			nil,
		},
		{
			"include signatures",
			[]config.Publisher{
				{
					Name:      "test",
					Signature: true,
					Cmd:       MockCmd + " {{ .ArtifactName }}",
					Env: []string{
						MarshalMockEnv(&MockData{
							AnyOf: []MockCall{
								{ExpectedArgs: []string{"a.deb"}, ExitCode: 0, ExpectedEnv: osEnv()},
								{ExpectedArgs: []string{"a.ubi"}, ExitCode: 0, ExpectedEnv: osEnv()},
								{ExpectedArgs: []string{"a.tar"}, ExitCode: 0, ExpectedEnv: osEnv()},
								{ExpectedArgs: []string{"a.sig"}, ExitCode: 0, ExpectedEnv: osEnv()},
								{ExpectedArgs: []string{"a.pem"}, ExitCode: 0, ExpectedEnv: osEnv()},
								{ExpectedArgs: []string{"foo/bar"}, ExitCode: 0, ExpectedEnv: osEnv()},
								{ExpectedArgs: []string{"foo/bar:amd64"}, ExitCode: 0, ExpectedEnv: osEnv()},
							},
						}),
					},
				},
			},
			nil,
		},
		{
			"docker",
			[]config.Publisher{
				{
					Name: "test",
					IDs:  []string{"img", "mnf"},
					Cmd:  MockCmd + " {{ .ArtifactName }}",
					Env: []string{
						MarshalMockEnv(&MockData{
							AnyOf: []MockCall{
								{ExpectedArgs: []string{"foo/bar"}, ExitCode: 0, ExpectedEnv: osEnv()},
								{ExpectedArgs: []string{"foo/bar:amd64"}, ExitCode: 0, ExpectedEnv: osEnv()},
							},
						}),
					},
				},
			},
			nil,
		},
		{
			"extra files",
			[]config.Publisher{
				{
					Name: "test",
					Cmd:  MockCmd + " {{ .ArtifactName }}",
					Env: []string{
						MarshalMockEnv(&MockData{
							AnyOf: []MockCall{
								{ExpectedArgs: []string{"a.deb"}, ExitCode: 0, ExpectedEnv: osEnv()},
								{ExpectedArgs: []string{"a.ubi"}, ExitCode: 0, ExpectedEnv: osEnv()},
								{ExpectedArgs: []string{"a.tar"}, ExitCode: 0, ExpectedEnv: osEnv()},
								{ExpectedArgs: []string{"a.txt"}, ExitCode: 0, ExpectedEnv: osEnv()},
								{ExpectedArgs: []string{"foo/bar"}, ExitCode: 0, ExpectedEnv: osEnv()},
								{ExpectedArgs: []string{"foo/bar:amd64"}, ExitCode: 0, ExpectedEnv: osEnv()},
							},
						}),
					},
					ExtraFiles: []config.ExtraFile{
						{Glob: filepath.Join("testdata", "*.txt")},
					},
				},
			},
			nil,
		},
		{
			"extra files with rename",
			[]config.Publisher{
				{
					Name: "test",
					Cmd:  MockCmd + " {{ .ArtifactName }}",
					Env: []string{
						MarshalMockEnv(&MockData{
							AnyOf: []MockCall{
								{ExpectedArgs: []string{"a.deb"}, ExitCode: 0, ExpectedEnv: osEnv()},
								{ExpectedArgs: []string{"a.ubi"}, ExitCode: 0, ExpectedEnv: osEnv()},
								{ExpectedArgs: []string{"a.tar"}, ExitCode: 0, ExpectedEnv: osEnv()},
								{ExpectedArgs: []string{"b.txt"}, ExitCode: 0, ExpectedEnv: osEnv()},
								{ExpectedArgs: []string{"foo/bar"}, ExitCode: 0, ExpectedEnv: osEnv()},
								{ExpectedArgs: []string{"foo/bar:amd64"}, ExitCode: 0, ExpectedEnv: osEnv()},
							},
						}),
					},
					ExtraFiles: []config.ExtraFile{
						{
							Glob:         filepath.Join("testdata", "*.txt"),
							NameTemplate: "b.txt",
						},
					},
				},
			},
			nil,
		},
		{
			"try dir templating",
			[]config.Publisher{
				{
					Name:      "test",
					Signature: true,
					IDs:       []string{"debpkg"},
					Dir:       "{{ dir .ArtifactPath }}",
					Cmd:       MockCmd + " {{ .ArtifactName }}",
					Env: []string{
						MarshalMockEnv(&MockData{
							AnyOf: []MockCall{
								{ExpectedArgs: []string{"a.deb"}, ExitCode: 0, ExpectedEnv: osEnv()},
							},
						}),
					},
				},
			},
			nil,
		},
		{
			"check env templating",
			[]config.Publisher{
				{
					Name: "test",
					IDs:  []string{"debpkg"},
					Cmd:  MockCmd,
					Env: []string{
						"PROJECT={{.ProjectName}}",
						"ARTIFACT={{.ArtifactName}}",
						"SECRET={{.Env.TEST_A_SECRET}}",
						MarshalMockEnv(&MockData{
							AnyOf: []MockCall{
								{
									ExpectedEnv: append(
										[]string{"PROJECT=blah", "ARTIFACT=a.deb", "SECRET=x"},
										osEnv()...,
									),
									ExitCode: 0,
								},
							},
						}),
					},
				},
			},
			nil,
		},
		{
			"override path",
			[]config.Publisher{
				{
					Name: "test",
					IDs:  []string{"debpkg"},
					Cmd:  MockCmd,
					Env: []string{
						"PATH=/something-else",
						MarshalMockEnv(&MockData{
							AnyOf: []MockCall{
								{
									ExpectedEnv: append(
										[]string{"PATH=/something-else"},
										osEnv("PATH")...,
									),
									ExitCode: 0,
								},
							},
						}),
					},
				},
			},
			nil,
		},
		{
			"command error",
			[]config.Publisher{
				{
					Disable: "true",
				},
				{
					Name: "test",
					IDs:  []string{"debpkg"},
					Cmd:  MockCmd + " {{.ArtifactName}}",
					Env: []string{
						MarshalMockEnv(&MockData{
							AnyOf: []MockCall{
								{
									ExpectedArgs: []string{"a.deb"},
									ExpectedEnv:  osEnv(),
									Stderr:       "test error",
									ExitCode:     1,
								},
							},
						}),
					},
				},
			},
			// stderr is sent to output via logger
			fmt.Errorf(`publishing: %s failed: exit status 1: test error`, MockCmd),
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("%d-%s", i, tc.name), func(t *testing.T) {
			err := Execute(ctx, tc.publishers)
			if tc.expectErr == nil {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.True(t, strings.HasPrefix(err.Error(), tc.expectErr.Error()), err.Error())
		})
	}
}
