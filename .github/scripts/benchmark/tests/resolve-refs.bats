#!/usr/bin/env bats

setup() {
	SCRIPT="${BATS_TEST_DIRNAME}/../resolve-refs.sh"

	export GITHUB_OUTPUT="${BATS_TEST_TMPDIR}/output"
	export REPO="example/repo"
	export GH_STUB_OUTPUT="${BATS_TEST_TMPDIR}/gh-output"

	: >"$GH_STUB_OUTPUT"

	# A gh that prints whatever the test put in GH_STUB_OUTPUT.
	mkdir -p "${BATS_TEST_TMPDIR}/bin"
	cat >"${BATS_TEST_TMPDIR}/bin/gh" <<'EOF'
#!/usr/bin/env bash
cat "$GH_STUB_OUTPUT"
EOF

	chmod +x "${BATS_TEST_TMPDIR}/bin/gh"

	export PATH="${BATS_TEST_TMPDIR}/bin:${PATH}"

	cd "$BATS_TEST_TMPDIR"
	git init --quiet --initial-branch=main .

	git config user.email "ci@example.com"
	git config user.name "CI"
	git config commit.gpgsign false
	git config tag.gpgsign false

	commit 172800
	OLD="$(git rev-parse HEAD)"

	commit 0
	HEAD_SHA="$(git rev-parse HEAD)"
}

# Commit an empty change dated the given number of seconds ago.
commit() {
	local -r date="@$(($(date +%s) - $1)) +0000"

	GIT_AUTHOR_DATE="$date" GIT_COMMITTER_DATE="$date" git commit --quiet --allow-empty --message "commit $1 seconds ago"
}

@test "self compares HEAD with itself" {
	run "$SCRIPT" self
	[ "$status" -eq 0 ]
	grep -qx "base_sha=${HEAD_SHA}" "$GITHUB_OUTPUT"
	grep -qx "head_sha=${HEAD_SHA}" "$GITHUB_OUTPUT"
}

@test "release resolves the latest release tag to its commit" {
	git tag v1.0.0 "$OLD"
	echo "v1.0.0" >"$GH_STUB_OUTPUT"

	run "$SCRIPT" release
	[ "$status" -eq 0 ]
	grep -qx "base_sha=${OLD}" "$GITHUB_OUTPUT"
}

@test "release skips a release that is HEAD" {
	git tag v1.0.0 "$OLD"
	git tag v1.1.0 "$HEAD_SHA"
	printf 'v1.1.0\nv1.0.0\n' >"$GH_STUB_OUTPUT"

	run "$SCRIPT" release
	[ "$status" -eq 0 ]
	grep -qx "base_sha=${OLD}" "$GITHUB_OUTPUT"
}

@test "release fails when no release is published" {
	run "$SCRIPT" release
	[ "$status" -eq 1 ]
	[[ "$output" == *"Could not find a published release"* ]]
}

@test "custom resolves BASE_REF" {
	git branch feature "$OLD"

	BASE_REF=feature run "$SCRIPT" custom
	[ "$status" -eq 0 ]
	grep -qx "base_sha=${OLD}" "$GITHUB_OUTPUT"
}

@test "custom resolves a branch that exists only on the remote" {
	git update-ref refs/remotes/origin/remote-only "$OLD"

	BASE_REF=remote-only run "$SCRIPT" custom
	[ "$status" -eq 0 ]
	grep -qx "base_sha=${OLD}" "$GITHUB_OUTPUT"
}

@test "custom fails for a ref that does not exist" {
	BASE_REF=missing run "$SCRIPT" custom
	[ "$status" -eq 1 ]
	[[ "$output" == *"Could not resolve missing to a commit"* ]]
}

@test "push on a branch uses the merge base with origin/main" {
	git update-ref refs/remotes/origin/main "$OLD"
	git checkout --quiet -b feature
	commit 0

	GITHUB_REF=refs/heads/feature BEFORE_SHA="$HEAD_SHA" run "$SCRIPT" push
	[ "$status" -eq 0 ]
	grep -qx "base_sha=${OLD}" "$GITHUB_OUTPUT"
}

@test "push on main uses the tip before the push" {
	commit 0

	GITHUB_REF=refs/heads/main BEFORE_SHA="$OLD" run "$SCRIPT" push
	[ "$status" -eq 0 ]
	grep -qx "base_sha=${OLD}" "$GITHUB_OUTPUT"
}

@test "push on main uses the parent of HEAD when the previous tip is not an ancestor" {
	git checkout --quiet --detach "$OLD"
	commit 60
	orphaned="$(git rev-parse HEAD)"
	git checkout --quiet main

	GITHUB_REF=refs/heads/main BEFORE_SHA="$orphaned" run "$SCRIPT" push
	[ "$status" -eq 0 ]
	grep -qx "base_sha=${OLD}" "$GITHUB_OUTPUT"
}

@test "push on main uses the parent of HEAD when there is no previous tip" {
	GITHUB_REF=refs/heads/main BEFORE_SHA="0000000000000000000000000000000000000000" run "$SCRIPT" push
	[ "$status" -eq 0 ]
	grep -qx "base_sha=${OLD}" "$GITHUB_OUTPUT"
}

@test "an unknown comparison fails" {
	run "$SCRIPT" weekly
	[ "$status" -eq 1 ]
	[[ "$output" == *"Unknown comparison: weekly"* ]]
}
