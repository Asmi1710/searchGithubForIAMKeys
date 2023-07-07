## Search Github For IAM ID Keys

Script in GoLang to identify and validate any AWS IAM keys embedded in the provided repository's code

## Approach

The problem has been broken down into parts and then the solution for each one of them provided, giving the final result. Following are the parts of the problem:
1. Authentication and creation of GitHub client so that we can access the desired repository
2. Getting the regex pattern for IAM Keys (access key ID and Secret Access Key) so that we can
use them for finding possible IAM Keys.
3. Getting the list of branches for the repository and then using loop to for processing each of
them:
a. Finding the content of each branch and then pattern matching is used to find possible
Keys.
b. Finding the list of commits and scanning each of them for the IAM Keys patterns
4. Using AWS API for the authentication of found possible matches for IAM Keys.
