# url-shortener
Proof-of-concept url shortener (to learn golang)

## Proposal

You've been asked to make an internal service for shortening URLs that anyone in the company can use. You can implement it in any way you see fit, including whatever backend functionality that you think makes sense. Please try to make it both end user and developer friendly. Please include a README with documentation on how to build, and run and test the system. Clearly state all assumptions and design decisions in the README. 

 

A short URL: 

    Has one long URL 

    This URL shortener should have a well-defined API for URLs created, including analytics of usage.

    No duplicate URLs are allowed to be created.

    Short links can expire at a future time or can live forever.


Your solution must support: 

    Generating a short url from a long url 

     Redirecting a short url to a long url. 

    List the number of times a short url has been accessed in the last 24 hours, past week, and all time. 

     Data persistence ( must survive computer restarts) 

     Metrics and/or logging: Implement metrics or logging for the purposes of troubleshooting and alerting. This is optional.

    Short links can be deleted


Project Requirements:

    This project should be able to be runnable locally with  some simple instructions

    This project's documentation should include build and deploy instruction

    Tests should be provided and able to be executed locally or within a test environment.

## Note
There's a [known bug](https://github.com/golang/go/pull/50339) in go's built-in reverse proxy library around trailing slashes. So for the purposes of this demo, I will require all destination URLs to support trailing/strict slashes.
