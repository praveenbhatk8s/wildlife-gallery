# NOTES — wildlife-gallery

Running log of what actually happened while building this. Raw material for the README's
"What went wrong" section, the blog post and LinkedIn posts. Newest first.

## 2026-10-26 — Phase 1: S3 → Lambda → CloudFront

**Result:** working end to end. Uploaded a falcon photo (`APFC.jpg`) to the originals bucket;
thumb and web versions served from CloudFront within seconds.

### What went wrong

- **`make upload FILE=~/Downloads/...` failed: "path does not exist".**
  zsh only expands `~` at the start of a word, so `FILE=~/...` reached make as a literal `~`,
  and `aws s3 cp` looked for a folder named `~`.
  Fix: use `$HOME/...`; the Makefile `upload` target can normalise it with
  `$(patsubst ~/%,$(HOME)/%,$(FILE))`.
  Lesson: shell expansion rules differ between shells and make; never assume `~` survives quoting.

- **Opened the originals bucket URL in a browser → `AccessDenied`.**
  Not a bug: the originals bucket is fully private by design (it holds files with GPS inside).
  Published photos are only reachable via CloudFront at `/photos/<photoId>/{thumb,web}.jpg`.
  Good screenshot for a post: "I tried to open my own bucket. AccessDenied. That's the point."

- **A unit test caught a real bug before deploy.**
  Camera name came out empty when EXIF had a `Make` but no `Model`. Rewrote the camera-name
  logic to handle maker-only, model-only, and models that repeat the maker ("Canon" + "Canon EOS R7").
  The test builds a JPEG with a real GPS EXIF block and asserts GPS is detected in the original
  and absent from both published sizes.

- **Resize test expected 1067 px, got 1066.** `imaging.Fit` rounds down (3000×2000 → 1600×1066).
  Fixed the expectation, not the code.

### Decisions made today

- Published images are re-encoded from pixels only, so they carry **no EXIF at all**
  (allow-nothing instead of deleting known GPS tags).
- Only a `hadGps` true/false flag is stored, never coordinates.
- Separate originals and derived buckets, so the function can never re-trigger itself.

### To verify / still open

- [ ] `exiftool` on the CloudFront `web.jpg` shows no GPS lines (screenshot for README)
- [ ] Check `hadGps` for the falcon photo; it's an edited `_HD` export, so the editor may already
      have stripped EXIF. Try a straight-from-camera JPEG.
- [ ] Push to GitHub, set Actions variables, create `aws-portfolio` environment with required reviewer
- [ ] First month's real cost from Cost Explorer (tag `repo = wildlife-gallery`)

### Post ideas

- "My bird photos were leaking nest locations, so I built a gallery that can't"
- The AccessDenied screenshot as a security-by-design hook
- `~` vs make: small, relatable, gets comments
