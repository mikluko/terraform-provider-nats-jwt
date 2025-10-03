# Import an existing operator key by seed
terraform import nsc_nkey.operator SOABC123DEF456GHI789JKL012MNO345PQR678STU901VWX234YZ

# Import an existing account key by seed
terraform import nsc_nkey.account SAXYZ789ABC012DEF345GHI678JKL901MNO234PQR567STU890VWX123

# Import an existing user key by seed
terraform import nsc_nkey.user SUJKL456MNO789PQR012STU345VWX678YZA901BCD234EFG567HIJ890

# The key type (operator/account/user) is automatically detected from the seed prefix
# No need to specify it in the import command or configuration
