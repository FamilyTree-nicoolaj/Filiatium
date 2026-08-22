class Filiatium < Formula
  desc "Validation et correction de GEDCOM 5.5.1 (compatibilité Gramps)"
  homepage "https://github.com/FamilyTree-nicoolaj/Filiatium"
  url "https://github.com/FamilyTree-nicoolaj/Filiatium/archive/refs/tags/v2.2.0.tar.gz"
  sha256 "f073cc86269e83d5f7d4a5f4da08dfd911939834c7b9f4860168911e568fbc93"
  license "MIT"
  depends_on "go" => :build
  depends_on "tesseract"
  # La formule "tesseract" seule n'embarque que eng/osd/snum : "tesseract-lang" ajoute
  # fra (entre autres), nécessaire à `filiatium import`.
  depends_on "tesseract-lang"

  def install
    system "go", "build", *std_go_args(ldflags: "-s -w -X main.version=v#{version}"), "."
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/filiatium --version")
  end
end
