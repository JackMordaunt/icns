tag=$1
current="$(git tag --points-at HEAD)"

if [[ -z "$tag" && -z $current ]]
then
    echo "commit not tagged and no tag provided"
    exit 1
fi

if [[ ! -z "$tag" && -z $current ]]
then
	git tag $tag -m "$tag"
fi

env GITHUB_TOKEN=$(pass goreleaser/github-token) goreleaser
