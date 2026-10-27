# loop-1 study tooling: product, image and containers

The Linux product the study builds once (`experiments/loop-1/script/build`), the image
every container runs, and the docker implementation of the port that runs a command in
an offline container (`experiments/loop-1/PREREG.md` §2, §7). The shell scripts are
covered by shellcheck.

## Product manifest

- **SP-2630~1**: WHEN a product manifest is read THE SYSTEM SHALL take from it the
  built commit, the working-tree flag, the Shallnot pin, the architecture, the SHA-256
  of every binary, the digest of every plugin tree and the four image digests, refuse
  an unknown member, trailing data, a malformed digest, an image not named by digest, an
  architecture outside arm64 and amd64, and a missing binary or plugin tree of the
  four-proofs arm, and write back exactly the manifest it read.
- **SP-2631~1**: WHEN a plugin tree is digested THE SYSTEM SHALL hash the paths and the
  content of its regular files alone, so that the digest does not depend on file modes
  or on the order the files were written, changes with any content or path change, and
  refuses a symbolic link.
- **SP-2632~1**: WHEN the product is described THE SYSTEM SHALL replace the plugin trees
  of the product directory with copies of the module plugins, the relay's, the umbrella's
  and the one of the pinned Shallnot tree, and write a manifest of the SHA-256 of every
  file of bin/ and the tree digest of every copied plugin, failing on a product without
  binaries or with a binary that is not a regular file.

## Docker box

- **SP-2633~1**: WHEN a command runs THE SYSTEM SHALL start it in a fresh named
  container with no network, the host user and group, every capability dropped, no new
  privilege, 2 CPUs, 3 GiB of memory with no more swap, and 1024 processes at most, and
  refuse a mounted path that holds `,` or `=` before starting any container.
- **SP-2634~1**: WHEN a command runs THE SYSTEM SHALL mount only its own mounts at their
  own paths and the product directory read-only at `/opt/musthold`, start the command
  under `env -i` with the command's environment alone, give each command a container of
  its own name, and never pass `--rm`.
- **SP-2635~1**: WHEN a command ends THE SYSTEM SHALL read from the container's state its
  exit code and its memory-kill flag and nothing else, return the client's output, and
  remove the container by name whatever happened, including a container that cannot be
  inspected and a cancelled context.
- **SP-2636~1**: WHEN a command outlives its limit THE SYSTEM SHALL kill its container
  and report the result as timed out, and WHEN it ends within its limit THE SYSTEM SHALL
  not kill it.
- **SP-2637~1**: WHEN a command runs THE SYSTEM SHALL start the docker client of the run
  under a wrapper that holds two machine-wide execution slots, waiting for them, and
  start the short control commands (inspect, kill, remove) outside it.

## Image

- **SP-2638~1**: WHEN the image definition is read THE SYSTEM SHALL name each source
  image by digest, take Claude Code from the npm package at the version of
  `script/claude-code.pin`, install pytest from a lock whose every requirement carries
  hashes.
- **SP-2639~1**: WHEN a command runs in the built image on the study's engine THE SYSTEM
  SHALL show it offline, running as the host user, with git, Python 3.13.14, pytest
  and Claude Code at the pinned version on its path, a read-only product directory and
  only the mounts it asked for.
