# Template-aware input design preview

Standalone, English-only Tabler prototype for a service port and a compound
container image. It has no repository requests, writes, local storage or
production-editor integration.

```sh
python3 -m http.server 8878 --bind 127.0.0.1 --directory docs/prototypes/template-input
```

Open http://127.0.0.1:8878.

## Review tasks

1. Confirm that **Template** renders references and surrounding text as a read-only composition.
2. Switch the image to **Raw value** and edit its exact source text.
3. Put the cursor inside the raw image and use **Insert variable**. It must insert at the cursor without materializing a value.
4. Load **Literal port**, choose **Template**, and select a variable. Port selection replaces the complete value.
5. Select an application and compare preview output with **Source value**. The source must stay unchanged.

## Contract boundary

`Template` and `Raw value` are derived UI modes, not YAML metadata. Both operate
on the same source string. One or more references, legacy `{{NAME}}` references
and surrounding text remain unchanged in YAML.

The display tokenizer and finite sample resolver are separate functions. The
resolver only illustrates states using fixtures; it is not production grammar,
scope discovery or backend evaluation.
