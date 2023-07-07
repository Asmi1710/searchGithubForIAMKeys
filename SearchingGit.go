package main

import (
	"context"
	"fmt"
	"log"
	"regexp"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
)

func main() {

	//Prompt the user to enter the GitHub access token
	fmt.Print("Enter your GitHub access token: ")
	var token string
	_, err_token := fmt.Scan(&token)
	if err_token != nil {
		log.Fatal(err_token)
	}

	// Prompt the user to enter the repository owner and name

	fmt.Print("Enter the repository owner: ")
	var owner string
	_, err_owner := fmt.Scan(&owner)
	if err_owner != nil {
		log.Fatal(err_owner)
	}

	fmt.Print("Enter the repository name: ")
	var repo string
	_, err_repo := fmt.Scan(&repo)
	if err_repo != nil {
		log.Fatal(err_repo)
	}

	// Prompt the user to enter the aws region

	fmt.Print("Enter the AWS region: ")
	var awsRegion string
	_, err_awsRegion := fmt.Scan(&awsRegion)
	if err_awsRegion != nil {
		log.Fatal(err_awsRegion)
	}

	// Create an authenticated GitHub client
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	// Compile the IAM key regular expression patterns
	iamKeyPattern := regexp.MustCompile(`AKIA[A-Z0-9]{16}`)
	iamKeyPattern2 := regexp.MustCompile(`[A-Za-z0-9/+=]{40}`)

	// Get the list of branches in the repository
	branches, _, err := client.Repositories.ListBranches(ctx, owner, repo, nil)
	if err != nil {
		log.Fatal(err)
	}

	counter := 0
	// Start recursive scanning for each branch
	for _, branch := range branches {
		branchName := branch.GetName()
		counter++
		fmt.Printf("(%d) Scanning branch: %s\n", counter, branchName)
		fmt.Println()
		fmt.Println("Scanning the code for the IAM Keys:--")
		// Start recursive scanning from the root directory for the branch
		if err := scanDirectory(ctx, client, owner, repo, branchName, "", iamKeyPattern, iamKeyPattern2, awsRegion); err != nil {
			log.Fatal(err)
		}

		// Get the commits for the branch
		commits, _, err := client.Repositories.ListCommits(ctx, owner, repo, &github.CommitsListOptions{
			SHA: branchName,
		})
		if err != nil {
			log.Fatal(err)
		}

		fmt.Println("Scanning the commits for the IAM Keys")

		// Start scanning the commits
		for _, commit := range commits {
			commitSHA := *commit.SHA
			fmt.Printf("Scanning commit with SHA : %s\n", commitSHA)

			// Get the commit details
			commit, _, err := client.Git.GetCommit(ctx, owner, repo, commitSHA)
			if err != nil {
				log.Fatal(err)
			}

			fmt.Println()
			fmt.Println()

			// Scan the commit message for access key ID and secret access key
			scanIAMKeys(commit.GetMessage(), iamKeyPattern, iamKeyPattern2, awsRegion)

			// Get the repository content at the commit
			_, contents, _, err := client.Repositories.GetContents(ctx, owner, repo, "", &github.RepositoryContentGetOptions{
				Ref: commitSHA,
			})
			if err != nil {
				log.Fatal(err)
			}

			for _, content := range contents {
				if *content.Type == "file" {
					fileData, err := content.GetContent()
					if err != nil {
						log.Fatal(err)
					}
					scanIAMKeys(fileData, iamKeyPattern, iamKeyPattern2, awsRegion)
				}
			}

		}

	}
}

func scanDirectory(ctx context.Context, client *github.Client, owner, repo, branch, path string, iamKeyPattern *regexp.Regexp, iamKeyPattern2 *regexp.Regexp, awsRegion string) error {
	opts := &github.RepositoryContentGetOptions{Ref: branch}

	// Get the contents of the current directory for the branch
	_, contents, _, err := client.Repositories.GetContents(ctx, owner, repo, path, opts)
	if err != nil {
		return err
	}

	// Scan each content item in the directory
	for _, content := range contents {
		if content != nil && content.GetType() == "file" {

			if *content.Path != "node_modules/.package-lock.json" && *content.Path != "package-lock.json" {
				fileContent, _, _, err := client.Repositories.GetContents(ctx, owner, repo, *content.Path, opts)
				if err != nil {
					log.Println(err)
					continue
				}

				contentData, err := fileContent.GetContent()
				if err != nil {
					log.Println(err)
					continue
				}

				matches1 := iamKeyPattern.FindAllString(contentData, -1)
				if len(matches1) > 0 {
					fmt.Printf("Possible IAM key(s):access key ID was found in file: %s (Branch: %s)\n", *content.Path, branch)
					for _, match := range matches1 {
						fmt.Println(match)
					}
					fmt.Println()
				}

				matches2 := iamKeyPattern2.FindAllString(contentData, -1)
				if len(matches2) > 0 {
					fmt.Printf("Possible IAM key(s):secret access key was found in file: %s (Branch: %s)\n", *content.Path, branch)
					for _, match := range matches2 {
						fmt.Println(match)
					}
					fmt.Println()
					fmt.Println("--------------------------TESTING Them using AWS APIs------------------------------")
					fmt.Println()
					fmt.Println()
				}

				if len(matches1) > 0 {
					if len(matches2) > 0 {
						for _, matchAccessKeyID := range matches1 {
							for _, matchSecretAccessKey := range matches2 {
								if isValidIAMKeys(matchAccessKeyID, matchSecretAccessKey, awsRegion) {
									fmt.Println("--------------------------SUCCESSFUL FINDING------------------------------")
									fmt.Printf("IAM key(s) found in file: %s (Branch: %s)\n", *content.Path, branch)
									fmt.Printf("AccessKeyID found %s \n", matchAccessKeyID)
									fmt.Printf("SecretAccessKey found %s \n", matchSecretAccessKey)
									fmt.Println()
									fmt.Println()
								}
							}
						}
					}
				}

			}
		} else if content != nil && content.GetType() == "dir" {
			// Recursively scan subdirectories
			err := scanDirectory(ctx, client, owner, repo, branch, *content.Path, iamKeyPattern, iamKeyPattern2, awsRegion)
			if err != nil {
				log.Println(err)
			}
		}
	}

	return nil
}

func scanIAMKeys(data string, iamKeyPattern1 *regexp.Regexp, iamKeyPattern2 *regexp.Regexp, awsRegion string) {
	tracker := false
	matches1 := iamKeyPattern1.FindAllString(data, -1)
	if len(matches1) > 0 {
		fmt.Println("Possible IAM key(s)(AccessKeyID) found:")
		for _, match := range matches1 {
			fmt.Println(match)
		}
		fmt.Println()
	}

	matches2 := iamKeyPattern2.FindAllString(data, -1)
	if len(matches2) > 0 {
		fmt.Println("Possible IAM key(s)(SecretAccessKey) found:")
		for _, match := range matches2 {
			fmt.Println(match)
		}
		fmt.Println()
	}

	if len(matches1) > 0 {
		if len(matches2) > 0 {
			for _, matchAccessKeyID := range matches1 {
				for _, matchSecretAccessKey := range matches2 {
					if isValidIAMKeys(matchAccessKeyID, matchSecretAccessKey, awsRegion) {
						tracker = true
						fmt.Println("--------------------------SUCCESSFUL FINDING------------------------------")
						fmt.Println("IAM key(s) found in the commits:")
						fmt.Printf("AccessKeyID found %s \n", matchAccessKeyID)
						fmt.Printf("SecretAccessKey found %s \n", matchSecretAccessKey)
						fmt.Println()
					}
				}
			}
		}
	}

	if tracker == false {
		tracker = true
		fmt.Println("--------------------------No IAM Key(s) found in this commit------------------------------")
		fmt.Println()
	}
}

func isValidIAMKeys(accessKeyID, secretAccessKey, awsRegion string) bool {
	// Create a new session using the IAM keys
	sess, err := session.NewSession(&aws.Config{
		Region:      aws.String(awsRegion), // Replace with the desired AWS region
		Credentials: credentials.NewStaticCredentials(accessKeyID, secretAccessKey, ""),
	})
	if err != nil {
		fmt.Println("--------------------------ERROR: AWS session failed------------------------------")
		fmt.Printf("Using accessKeyID: %s & secretAccessKey: %s, Error creating AWS session: %s \n", accessKeyID, secretAccessKey, err)
		fmt.Println()
		return false
	}

	// Create an STS (Security Token Service) client
	svc := sts.New(sess)

	// Call the GetCallerIdentity API to validate the IAM keys
	_, err = svc.GetCallerIdentity(&sts.GetCallerIdentityInput{})
	if err != nil {
		fmt.Println("--------------------------ERROR: AWS API failed------------------------------")
		fmt.Printf("Using accessKeyID: %s & secretAccessKey: %s, Error calling AWS API: %s \n", accessKeyID, secretAccessKey, err)
		fmt.Println()
		fmt.Println()
		return false
	}

	return true
}
