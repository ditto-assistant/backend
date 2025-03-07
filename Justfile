set windows-shell := ["powershell.exe", "-NoLogo", "-Command"]

@local: generate
    echo "Running on http://localhost:3400"
    go run main.go

generate:
    templ fmt .
    templ generate

staging:
    DITTO_ENV=staging go run main.go

deploy:
	gcloud run deploy --port 3400 \
		--set-env-vars GCLOUD_PROJECT=ditto-app-dev \
		--set-env-vars GCLOUD_LOCATION=us-central1 \
		--source . \

kill:
	lsof -i :3400 | grep LISTEN | awk '{print $2}' | xargs kill

db *ARGS:
	go run cmd/dbmgr/dbmgr.go {{ARGS}}

install-dbmgr:
	go install cmd/dbmgr/dbmgr.go

search *ARGS:
	go run cmd/dbmgr/dbmgr.go search {{ARGS}}

build:
	docker build -t ditto-backend .

push-tag-number:
	git tag -a $VERSION -m "Release $VERSION"
	git push origin $VERSION

# Shared implementation for incrementing version parts
# part: which part to increment (major, minor, patch)
# dry-run: whether to only show what would be done
push-new-version part="patch" dry-run="false":
    #!/usr/bin/env bash
    # Get the latest tag from GitHub
    LATEST_TAG=$(git describe --tags `git rev-list --tags --max-count=1`)
    
    # Check if the tag follows the expected format v{major}.{minor}.{patch}
    if [[ ! $LATEST_TAG =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
        echo "Warning: Latest tag '$LATEST_TAG' doesn't match expected format v{major}.{minor}.{patch}"
        echo "Using v0.0.0 as base version"
        MAJOR=0
        MINOR=0
        PATCH=0
    else
        # Extract major, minor, patch from tag
        IFS='.' read -r VMAJOR MINOR PATCH <<< "$LATEST_TAG"
        # Remove 'v' prefix from major part
        MAJOR=${VMAJOR#v}
    fi
    
    # Increment appropriate version part based on parameter
    if [[ "{{part}}" == "major" ]]; then
        NEW_MAJOR=$((MAJOR + 1))
        NEW_MINOR=0
        NEW_PATCH=0
        NEW_TAG="v$NEW_MAJOR.$NEW_MINOR.$NEW_PATCH"
    elif [[ "{{part}}" == "minor" ]]; then
        NEW_MAJOR=$MAJOR
        NEW_MINOR=$((MINOR + 1))
        NEW_PATCH=0
        NEW_TAG="v$NEW_MAJOR.$NEW_MINOR.$NEW_PATCH"
    else  # Default to patch
        NEW_MAJOR=$MAJOR
        NEW_MINOR=$MINOR
        NEW_PATCH=$((PATCH + 1))
        NEW_TAG="v$NEW_MAJOR.$NEW_MINOR.$NEW_PATCH"
    fi
    
    if [[ "{{dry-run}}" == "true" ]]; then
        echo "Dry run mode - would create and push tag: $NEW_TAG"
    else
        # Create and push new tag
        git tag -a $NEW_TAG -m "Release $NEW_TAG"
        git push origin $NEW_TAG
    fi

# Wrapper commands for specific version increments
push-new-tag dry-run="false":
    just push-new-version patch {{dry-run}}

push-new-major-tag dry-run="false":
    just push-new-version major {{dry-run}}

push-new-minor-tag dry-run="false":
    just push-new-version minor {{dry-run}}

# get the latest tag
@version:
	git describe --tags `git rev-list --tags --max-count=1`

# create a github release for the latest tag with auto-generated release notes
gh-release:
	#!/bin/sh
	VERSION=$(just version)
	gh release create $VERSION --generate-notes

# create a new release for the latest tag with auto-generated release notes
create-patch-release: push-new-tag gh-release
alias cpr := create-patch-release

# create a new major release with auto-generated release notes
create-major-release: push-new-major-tag gh-release
alias cmr := create-major-release

# create a new minor release with auto-generated release notes
create-minor-release: push-new-minor-tag gh-release
alias cnr := create-minor-release

# create a release with a custom tag
create-release TAG:
    #!/bin/sh
    # Create and push the tag
    git tag -a {{TAG}} -m "Release {{TAG}}"
    git push origin {{TAG}}
    # Create the GitHub release
    gh release create {{TAG}} --generate-notes

# Run 0.11 migration on single user
@migrate-11-single EMAIL:
    just db -log debug firebase mem delcol -email {{EMAIL}} -col embedding
    just db -env prod -log debug firebase mem embed -email {{EMAIL}} -fields prompt,embedding_prompt_5,response,embedding_response_5 -model-version 5

# Run 0.11 migration on all users
@migrate-11-all:
    just db -env prod firebase mem delcol -all-users -col embedding
    just db -env prod firebase mem embed -all-users -fields prompt,embedding_prompt_5,response,embedding_response_5 -model-version 5