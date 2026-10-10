import json, sys, jsonschema
schema = json.load(open(sys.argv[1]))
doc = json.load(open(sys.argv[2]))
v = jsonschema.Draft4Validator(schema, format_checker=jsonschema.Draft4Validator.FORMAT_CHECKER)
errs = list(v.iter_errors(doc))
for e in errs:
    print("ERROR", list(e.absolute_path), e.message)
print("errors:", len(errs))
