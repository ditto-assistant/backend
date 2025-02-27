set windows-shell := ["powershell.exe", "-NoLogo", "-Command"]

local:
	go run main.go

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

push-new-tag dry-run="false":
    #!/usr/bin/env bash
    # Get the latest tag from GitHub
    LATEST_TAG=$(git describe --tags `git rev-list --tags --max-count=1`)
    
    # Extract major, minor, patch from tag (assuming format like v1.2.3)
    IFS='.' read -r MAJOR MINOR PATCH <<< "${LATEST_TAG#v}"
    
    # Increment patch version
    NEW_PATCH=$((PATCH + 1))
    
    # Create new tag with incremented patch version
    NEW_TAG="v$MAJOR.$MINOR.$NEW_PATCH"
    
    if [[ "{{dry-run}}" == "true" ]]; then
        echo "Dry run mode - would create and push tag: $NEW_TAG"
    else
        # Create and push new tag
        git tag -a $NEW_TAG -m "Release $NEW_TAG"
        git push origin $NEW_TAG
    fi

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

# Run 0.11 migration on single user
@migrate-11-single EMAIL:
    just db firebase mem delcol -email {{EMAIL}} -col embedding
    just db -env prod firebase mem embed -email {{EMAIL}} -fields prompt,embedding_prompt_5,response,embedding_response_5 -model-version 5

@migrate-11-all:
    just db -env prod firebase mem embed -all-users -fields prompt,embedding_prompt_5,response,embedding_response_5 -model-version 5