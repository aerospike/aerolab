V=$(git branch --show-current); if [[ $V == v* ]]; then printf ${V:1} > ../VERSION.md; fi; cat ../VERSION.md > embed_branch.txt