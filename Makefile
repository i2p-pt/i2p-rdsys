translations:
	@goi18n extract -format yaml -outdir locales
	@goi18n merge -format yaml -outdir locales active.en.yaml translate.de.yaml
