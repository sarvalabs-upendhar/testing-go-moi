[coco]
version = "0.3.1"

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
