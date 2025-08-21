#!/bin/bash
# Fix whitespace issues

# Get the list of files with whitespace issues
files=$(git diff --cached --check | grep -o "^[^:]*" | sort -u)

# Process each file
for file in $files; do
  echo "Processing $file"
  # Remove trailing whitespace
  sed -i 's/[[:space:]]*$//' "$file"
  # Add back to index
  git add "$file"
done

echo "All whitespace issues fixed!"
