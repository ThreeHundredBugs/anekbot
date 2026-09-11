.PHONY: test

test:
	pytest .

artifact_path ?= "$(CURDIR)/anekbot.zip"
build:
	rm "$(artifact_path)" || true
	zip -r "$(artifact_path)" . -x "*_tests.py" "*__pycache__*" "*pytest*" "*.venv*" "test-*" && \
	du -h "$(artifact_path)" && \
	unzip -l "$(artifact_path)"
