#!/bin/bash

# Usage: generate_patch_release_notes.sh
#
# Generates and creates commits for GCP CSI driver patch release notes.
#
# Required environment variables:
# CSI_RELEASE_TOKEN: Github token needed for generating release notes
#
# Prerequisites:
# - This script creates and deletes the origin/changelog branch in your git
# workspace. Make sure you are not using the branch for anything else.
#
# Instructions:
# 1. Update the versions in the $releases array
# 2. Set environment variables
# 3. Run the script
# 4. The script pushes the commits to your origin/changelog branch
# 5. Make any modifications as needed
# 6. Create a PR
#
# Caveats:
# - This script doesn't handle regenerating and updating existing branches.
# - The "--start-rev" option in the release-notes generator is inclusive, which
# causes the last commit from the last patch to show up in the release notes (if
# there was a release note). This needs to be manually modified until a solution is found.

set -e
set -x

repo="gcp-compute-persistent-disk-csi-driver"
releases=(
  "1.12.5"
  "1.11.7"
  "1.10.12"
  "1.9.14"
  "1.8.18"
  "1.7.19"
)

function gen_patch_relnotes() {
  rm out.md || true
  rm -rf /tmp/k8s-repo || true
  GITHUB_TOKEN=$CSI_RELEASE_TOKEN \
  release-notes --start-rev=$3 --end-rev=$2 --branch=$2 \
    --org=kubernetes-sigs --repo=$1 \
    --required-author="" --markdown-links --output out.md
}

script_dir="$(dirname "$(readlink -f "$0")")"
pushd "$script_dir/../CHANGELOG"

# Create branch
git fetch upstream
git checkout master
git rebase upstream/master

branch="changelog"
if [ `git rev-parse --verify "$branch" 2>/dev/null` ]; then
  git branch -D "$branch"
fi
git checkout -b $branch

for version in "${releases[@]}"; do
  # Parse minor and patch version
  minorPatchPattern="(^[[:digit:]]+\.[[:digit:]]+)\.([[:digit:]]+)"
  [[ "$version" =~ $minorPatchPattern ]]
  minor="${BASH_REMATCH[1]}"
  patch="${BASH_REMATCH[2]}"

  echo $repo $version $minor $patch
  newVer="v$minor.$patch"
  prevPatch="$(($patch-1))"
  prevVer="v$minor.$prevPatch"

  # Generate release notes
  gen_patch_relnotes $repo release-$minor $prevVer
  cat > tmp.md <<EOF
# $newVer - Changelog since $prevVer

EOF

  cat out.md >> tmp.md
  echo >> tmp.md
  echo >> tmp.md

  file="CHANGELOG-$minor.md"
  cat $file >> tmp.md
  mv tmp.md $file

  git add -u
  git commit -m "Add changelog for $version"
done

git push -f origin $branch

popd
