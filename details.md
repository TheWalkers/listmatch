listmatch design and motivation
-------------------------------

**HEY:** This is here to start describing why listmatch exists and how it works, but some stuff described here, like the Web uploader, isn't there yet. Not every bit of plan/wording/thinking is final.

How's it work?
==============

Say two organizations, you and your partner, want to know the overlap between your email lists. Listmatch lets you do this securely using a neutral third-party server:

1. You pick a random key, hash each email addresses with the key, and send the hashes to the server. You *don't* give the server the key.
2. You send your partner the key, they hashes _their_ email addresses with it, send the hashes to the server.
3. The server compares the files of hashes. Because it doesn't have the key, it can't even guess at what emails were hashed, but it can come up with info "your 1st, 3rd, and 5th hashes had matches in your partner's file". 
4. You and your partner go back to the server download the info on what items matched.

Now you and your partner know which emails had a match in the other's file. Neither of you finds out about the entries that *didn't* match, because they didn't receive those hashes. And the server can't try hashing random email addresses because it doesn't have the key. 

We think this is a big practical improvement over the old practice of exchanging hashed emails directly, and several variations trying to improve it. Below, we go into why at more length.


Why's it needed?
================

Folks might wonder why you'd use listmatch rather than other ways to match lists. Here are some things people use or have discussed using, and where we think they have issues.

**Exchanging hashes directly.** The problem here is that though the attacker can't "work backwards" from the hashes directly, they can make a ton of guesses quickly, try them all, and recover most of your items. Specialized cracking software can hash billions of values per second on a GPU, and it's inexpensive to get a cluster with lots of GPUs as cloud instances, especially the cheaper "spot" or "preemptible" kind. 

It's not primarily the length of the hashes or the security of the hash function that determines how hard it is to recover items with this type of brute-force; it's how hard-to-guess the raw values you're hashing are. And it's not hard to guess emails: lists of tens of millions of addresses are already public to check against, and you can try common patterns like `[firstname.lastname]@[domain]` with common first names, last names, and domains. You can do more than that with billions of hashes per second at your disposal, but we're not trying to write out the steps to an attack, just to say that powerful attacks are possible.

So hashing alone doesn't effectively hide most types of private data from a smart attacker.


**Using a central server for hashing.** This is *not* an improvement on you and your partner each hashing lists on your own machines. Even if the upload form claims it only hashes the data you send, and even if there's published source code for a server that just hashes, fundamentally you're sending your raw list to machines operated by someone else. You can't *know* what code someone else's machines are running; if the operator's intentions were good but the machine was compromised, whoever did the compromising still gets your raw, unhashed list. (If you're running the hashing process on your machine, by contrast, you *can* see what's running, via tools like your browser's debugger, and any attacker modifying the hashing process or copying the raw file at least theoretically risks being caught.)


**Exchanging keyed/salted hashes directly.** Password databases these days have a unique 'salt' value for every user. This can be a significant help if the password database is leaked: instead of hashing one password guess and checking if *anyone* in the database has that password, the attacker has to try each guess for each of the users. If the list has a million users, that makes a big difference!

Sadly, we can't use a unique salt for every value when list-matching, because the whole goal here is that the same input always hashes to the same output, so that we can match up the identical values in the lists. You *can* still use a single key/salt for the whole list. Then, at least, attackers can't use a precomputed list of guesses (a rainbow table) to speed up their search. It also lets you, say, email the hashes and SMS someone the key, and hope no attacker obtains both.

But the *recipient* who has both the hashes and the key could still brute-force efficiently at the high hash rates GPUs allow. This seems like a problem, since usually the premise of list matching is 'we aren't sending them the list'. If the partner were malicious, or if the key and hashes were both compromised in a breach, it wouldn't be hard to brute-force a lot of those keyed hashes back into raw data.


**Exchanging slow hashes directly.** There are functions *designed* to run slowly, often used to hash passwords to make brute-forcing slower. PBKDF2, bcrypt, scrypt, and Argon2 are examples. They're usually used in combination with salting to make it much harder for brute-forcers to take much advantage of a dumped password database. They just make the process slower for *everyone*, since it's usually fine if it takes, say, a tenth of a second to check a user's password when they log in. 

I think slow hashes aren't quite as great a fit for list-matching. First, a slowdown factor (work factor) that's good for passwords is painfully slow for list-matching. You check passwords one at a time, so a 0.1 second delay is no big deal. But when list-matching you may hash, say, ten million rows at once, so at 0.1 second/hash you'd need more than a CPU-week's worth of hashing power. So you either have to make hashing your list a very compute-intensive operation or (more likely) lower the work factor. Second, with a slowed-down hash the end result will *still* be easier to brute-force than a password database: you don't have per-item salts to further multiply the work required to brute-force, and many email addresses are easier to guess than passwords.

Slowing the hash somewhat (say, to 1ms not 100ms) could slightly slow brute-forcing for someone that got both the hash and the key, and the cost could be bearable for some (each million items would take about 17 CPU-minutes). However, it's still pretty annoying to legitimate users with larger lists, and it only somewhat helps with the brute-forcing problem here; it's nothing near as helpful as it is for password hashing. Given a good third-party matching setup I'd argue slowed-down hashing is no longer the right tradeoff.


Data handling benefits
======================
The discussion so far is mostly in terms of how other methods can be brute-forced. But another way to think about the problem is in terms of good practices handling sensitive data. There are a few ways that a third-party match setup can help you with that:

* **Deleting hashes automatically.** A properly configured server can delete uploads after a set period of time, automatically. If you manually send files, it's hard to know if you *and your partner* both deleted every copy from email/Dropbox, trash folders, local copies on their laptop, etc.
* **Keeping data off laptops.** The command-line utility makes it easy to hash and upload directly from your server without copying the sensitive data to anyone's laptop. The less data that touches a laptop, the less at risk if it's lost or stolen.
* **Keeping key and hashes apart.** Unlike if you're manually exchanging a list of keyed hashes, third-party matching sends the hashes one place (to the server over HTTPS) and the key to another (your match partner). That means there's no single place an attacker could get them both.
* **Minimal surface area for attacking the server.** The server is a self-contained program with minimal dependencies that can run on a cheap, locked-down, isolated cloud instance. Less to manage can make it simpler for server operators to get server security right.


Usage advice
============

The two main things to remember to use listmatch safely are:

* **Use a third-party server.** A neutral server operator who wouldn't collude with either party to leak data is what makes this work. If a few habitual listmatch users run servers, that's not hard to achieve: when A exchanges with B they use C's server; when B exchanges with C they use A's. 

* **Be sure you're using a legit uploader.** If you feed your list to a compromised uploader, it's gone. When using a web uploader, you *have* to get it served securely by someone you trust. You can get the command-line tool as source or binaries, to easily see what you're running. More about this below.


Knowing your uploader
=================

You need to choose between the web and command-line uploaders for listmatch. I think the decision will be easy for most, but it's worth talking about the tradeoffs.

Like with any website you use to handle sensitive data--Google Docs, Dropbox, anything--you need some level of confidence that the Web uploader doing what it says it will, that it's not intentionally misbehaving, profoundly broken, or subverted by someone else, and that you're using the legitimate site rather than using an imitation (getting phished). This is part of why you *have* to use Web uploaders securely served from trustworthy sources. Check that address bar.

One helpful thing about listmatch's web uploader in particular is that it does the hashing *on your computer*, and you can actually see the code that's doing the hashing (through, for example, your browser's debugging tools). Most users won't do this, but the *fact* that users can see the code they're running at least means that anyone running a compromised Web uploader would risk discovery. 

If you're a programmer, it's a relatively simple thing to reassure yourself the command-line uploader does what it says: you can check out the source, review it, and compile it yourself.

For perspective, though: this isn't an issue specific to listmatch; there is a lot of software we trust with important data. And the older procedure it replaces (exchanging hashes) makes it easy for anyone who obtains the data to reconstruct your list _even when everything goes well_, not only in worst-case scenarios where an attacker has created sabotaged versions of webpages or such. And the most important defense is just good practice anyway: use a securely hosted page at a site you trust whenever you're handling important data.

Anyway, here's how I think it breaks down for most:

**Less technical users: consider the Web uploader.** Some users don't know how to use a command-line tool, so the Web uploader is the safest option available. Relatedly, if you're uneasy installing unfamiliar programs on your computer just because a list-matching partner asked you to--a reasonable unease!--the Web uploader works around that.

**More technical users, or those with multi-million-email lists: consider the command-line uploader.** Users with multi-million-row lists and/or lots of Unix know-how may want the command-line tool: it lets you initiate a match directly from your server's command line, so you never download the list to a laptop and can use a better connection when interacting with the server. Hashing is a bit faster, too.



Implementation decisions in listmatch
=====================================

This section is more for the sort of geeks who would implement something like this themselves, and are curious about why listmatch made specific choices. In explaining decisions I'm not trying to deny other choices would have worked as well--they likely would have!

_Why call the secret the 'key'?_ We could have called it 'salt', but 'key' conveys to users that it's a secret. Discouraging users from disclosing the secret seemed like the most important thing here.

_Why truncate hashes to 64 bits?_ It's the 256-bit key, not the output length, that makes it infeasible for the server to reconstruct raw inputs by brute-force guessing. The 64 bit tags are easily long enough to keep accidental false positives from being a problem: matching two lists of 100 million, there's less than a 0.1% chance of one accidental collision. 

Shortening the values improves performance and simplicity: uploading even 64 bits is slower than hashing on some CPU/connection combinations, so we'd rather not quadruple the amount we need to upload. Shrinking the upload also helps the server fit entire lists in memory for efficient matching, even on a small cloud instance. Finally, it simplifies the server's code to use hashes small enough to fit in a built-in data type, and simpler code is easier to review or make an independent implementation of.

_Why use binary?_ Similar to why we truncate to 64 bits: when upload speed can be the bottleneck, representing the hashes as compactly as possible is good. It might be a little less familiar than dealing in text in, say, JavaScript, but it's well supported by modern engines.

_Why use SHA-256?_ For this use case, SHA-256 is probably even overkill: all we need from a hash function is that there be no faster way to 'work backwards' from a hash to the input than just guessing possible inputs (including key). That's preimage resistance, which is much less strict than collision-resistance. Something like SipHash might be sufficient for our needs, but we're using SHA-256 because it's more than strong enough, well known, and fast enough that hashing isn't necessarily the bottleneck, including in JavaScript. 

_Why so many options?/Why not more options?_ We think the CLI and Web uploaders are each best for different sorts of user, and that it's best not to force everyone to use particular uploaders or servers. At the same time, we realize lots of options can intimidate less technical users, so we tried to provide easy defaults and avoid adding other options we didn't need.

_Why is the server/CLI in Go?_ Definitely doesn't have to be, and other implementations are welcome! It does make it easy to distribute binaries anyone can run on various OSes/versions/configuration, and on the server side it made it easy to implement goodies like automatic HTTPS setup via Let's Encrypt.


Other fun ideas
===============

*A non-Web-based GUI uploader.* For the less technically savvy, I'm not sure that a downloadable executable is actually a good thing, because of the risk the user is tricked into installing something malicious. If using a malicious webpage is bad, running a malicious executable locally seems _worse_ since executables have much broader power by default. All the same, people apparently downloaded and ran hashing tools before, and if they want to continue to do so, someone could provide an option that looks like that.

*Using your DB server to do the hashing.* Uploaders could accept files where the first column is the keyed SHA256 hash, instead of the raw email address, and could provide SQL for generating hashes using MySQL, Postgres, etc. The SQL would have to be customized, and there's some fiddliness making sure the (binary) key is hashed raw rather than, say, UTF-8 encoded. And this doesn't fully eliminate an evil uploader's ability to do mischief (it could still give away the key), but it does keep the uploader from handling raw files. Given all the caveats unsure it's worth the tradeoff, but maybe someone would be interested in it.

*Integrations.* You can imagine integrations to streamline the multi-step process of exporting/uploading/matching. The first step is just to get something working and, hopefully, used, though.
