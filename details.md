listmatch design and motivation
-------------------------------

**HEY:** This is here to start describing why listmatch exists and how it works, but some stuff described here, like the Web client, isn't there yet. Not every bit of plan/wording/thinking is final.

How's it work?
==============

Say two organizations, client 1 and client 2, want to know the overlap between their email lists. Here's how they match lists with a neutral server's help.

1. Client 1 picks a random key. They hash each of their email addresses together and send the hashes to the server. It does *not* send the server the key.
2. Client 2 gets the key from client 1, hashes their email addresses with the same key, and sends the hashes to the server.
3. The server compares the hashes. Now, for each file it has information like "the 1st, 3rd, and 5th hashes had matches in the other file". 
4. The clients go back to the server download that info.

Now each client knows which entries in their files matched the other client's data. Neither client finds out about the entries that *didn't* match, because they didn't receive those hashes. And the server can't try hashing random email addresses because it doesn't have the key. Preventing anyone from doing this kind of brute-force attack is a significant win over just sending email addresses hashed without a key; if you do that, there are tools that let an adversary guess billions(!) of possible addresses per second. Much more aobut that in "Why's it needed?" below.

Each client needs to be reasonably confident that the server really *is* a neutral party, and doesn't conspire with the other client to use the key and hashes together for a brute-force attack. They also need to be confident the code they provide their list to (their client program) really implements the protocol; if you _think_ you're using your list in this kind of match, but you're really uploading it to phishingsite.org, obviously all bets are off.


Usage advice
============

The assumptions above translate directly into some advice for using listmatch:

* Use a third-party server. A neutral server operator who wouldn't collude with either party to leak data is what makes this work. If a few habitual listmatch users run servers, that's not hard to achieve: when A exchanges with B they use C's server; when B exchanges with C they use A's. 

* Be sure you're using a legit hashing client. If you feed your list to a compromised client, it's gone. You can download the web client, or get the command-line tool as source or binaries. If you use a web client, you *have* to get it served securely by someone you trust. 


Choosing a client
=================

You need to choose between the web and command-line clients for listmatch. 

Like with any website you use to handle sensitive data--Google Docs, Dropbox, anything--you need some level of confidence that the Web client doing what it says it will, that it's neither intentionally misbehaving or subverted by someone else. This is part of why you *have* to use Web clients securely served from trustworthy sources. 

One reassuring thing about listmatch's web client in particular is that it does the hashing *on your computer*, and you can actually see the code that's doing the hashing (through, for example, your browser's debugging tools). Most users might not actually do this, but the *fact* that users can see the code they're running at least means that anyone running a compromised Web client would risk discovery. 

Practically speaking, I think the choice is simple for most. 

Some users don't know how to use a command-line tool, so the Web client is the safest option available. Relatedly, if you're uneasy installing unfamiliar programs on your computer just because a list-matching partner asked you to--a reasonable unease!--the Web client works around that.

Users with multi-million-row lists and/or lots of Unix know-how may want the command-line tool: it lets you initiate a match directly from your server's command line, so you never download the list to a laptop and can use a better connection when interacting with the server. Hashing is a bit faster, too.


Why's it needed?
================

Folks might wonder why you'd use listmatch rather than other ways to match lists. (`listmatch` is for email lists, but the more general problem could apply to lists of anything--names, phone numbers, etc.) Here are some existing approaches and ideas, and their downsides and limitations:

*Exchanging hashes directly.* The problem here is that though the attacker can't "work backwards" from the hashes directly, they can make a ton of guesses quickly, try them all, and recover most of your items. If you're hashing, say, US phone numbers, an attacker can run through every possible value trivially. Specialized cracking software can hash billions of values per second on a GPU, and it's inexpensive to get a cluster with lots of GPUs as cloud instances, especially the cheaper "spot" or "preemptible" kind. 

It's not primarily the length of the hashes or the security of the hash function that determines how hard it is to recover items with this type of brute-force; it's how hard-to-guess the raw values you're hashing are. For many types of private info--names, street or email addresses, etc.--an attacker can't guess every possibility, but can guess values they've seen in data elsewhere; they can easily generate well-informed guesses for values they don't have in a list, like combining common first and names from U.S. census data. What's more, before they even obtain your hashes, attackers can prepare a huge table of guesses, a 'rainbow table', to make the process faster later.

So hashing alone doesn't effectively hide most types of private data from a smart attacker.


*Using a central server for hashing.* This is *not* an improvement on the two parties each hashing lists on their own machines. Even if the upload form claims it only hashes the data you send, and even if there's published source code for a server that just hashes, fundamentally you're sending your raw list to machines operated by someone else. You can't *know* what code someone else's machines are running; if the operator's intentions were good but the machine was compromised, whoever did the compromising still gets your raw, unhashed list. Compare to the "Choosing a client" advice above: you *can* check what code is running on your machine, so anyone serving up modified clients at least theoretically risks being caught.


*Exchanging keyed/salted hashes directly.* Password databases these days have a unique 'salt' value for every user. This can be a significant help if the password database is leaked: instead of hashing one password guess and checking if *anyone* in the database has that password, the attacker has to try each guess for each of the users. If the list has a million users, that makes a big difference!

Sadly, we can't use a unique salt for every value when list-matching, because the whole goal here is that the same input always hashes to the same output, so we can match up the identical values in the lists. You *can* still use a single key/salt for the whole list. That helps *somewhat*: attackers can't use a precomputed list of guesses (a rainbow table) to speed up their search, and you could, say, email the hashes and SMS someone the key, and hope no attacker obtains both.

But the *recipient* who has both the hashes and the key could still brute-force efficiently at the high hash rates GPUs allow. This seems like a problem, since usually the premise of list matching is 'we aren't sending them the list'. If the partner were malicious, or if the key and hashes were both compromised in a breach, it wouldn't be hard to brute-force a lot of those keyed hashes back into raw data.


*Exchanging slow hashes directly.* There are functions *designed* to run slowly, often used to hash passwords to make brute-forcing slower. PBKDF2, bcrypt, scrypt, and Argon2 are examples. They're usually used in combination with salting to make it much harder for brute-forcers to take much advantage of a dumped password database. They just make the process slower for *everyone*, since it's usually fine if it takes, say, a tenth of a second to check a user's password when they log in. 

I think slow hashes aren't quite as great a fit for list-matching. First, a slowdown factor (work factor) that's good for passwords is painfully slow for list-matching. You check passwords one at a time, so a 0.1 second delay is no big deal. But when list-matching you may hash, say, ten million rows at once, so at 0.1 second/hash you'd need more than a CPU-week's worth of hashing power. So you either have to make hashing your list a very slow operation or (more likely) lower the work factor. Second, with a slowed-down hash the end result will *still* be easier to brute-force than a password database: you don't have per-item salts to further multiply the work required to brute-force, and some types of hashed data like phone numbers may be easier to guess than good passwords.

Slowing the hash somewhat (say, to 1ms not 100ms) could slightly slow brute-forcing for someone that got both the hash and the key and could be bearable for some (each million items would take about 17 CPU-minutes). However, it's still pretty annoying to legitimate users with larger lists, and it only somewhat helps with the brute-forcing problem here; it's nothing near as helpful as it is for password hashing. Given a good third-party matching setup I'd argue slowed-down hashing is no longer the right tradeoff.


*Practical considerations.* Distinct from solving the problems with hashing, a good list-matching setup can also help avoid leaving unnecessary copies of sensitive data sitting around. 

If you send a file of hashes over email, Dropbox, etc., it might stick around beyond its useful lifetime, in their original locations, in trash folders, or on laptops. And even if *you* do everything right, the other organization you're working with might not. You don't want that data laying around: an otherwise boring Dropbox account can become an important target if it has hashes of all your email addresses. 

Having a server that automatically deletes hashes after a specified time period can reduce the window where they could even theoretically be found by attackers. Using the command-line tool on your server can also avoid ever having to handle raw email addresses on a laptop, removing another risky step.


Why this protocol?
==================

TK


Other fun ideas
===============

TK