.PHONY: lint format lint-fix

lint:
	$(MAKE) -C v2 lint

format:
	$(MAKE) -C v2 format
	$(MAKE) -C execution format

lint-fix:
	$(MAKE) -C v2 lint-fix
