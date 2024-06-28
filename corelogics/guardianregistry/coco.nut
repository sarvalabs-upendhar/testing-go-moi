[coco]
version = "0.5.0"

[module]
name = "GuardianRegistry"
version = "0.3.0"
license = []
repository = "https://github.com/sarvalabs/go-moi/compute/corelogics/guardian_registry"
authors = ["Manish Meganathan"]

[target]
os = "MOI"
arch = "PISA"

[target.moi]
format = "YAML"
output = "guardians"

[target.pisa]
format = "ASM"

[lab.render]
big_int_as_hex = true
bytes_as_hex = false

[lab.config.default]
url = "http://127.0.0.1:6060"
env = "main"
